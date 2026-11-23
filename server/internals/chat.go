package internals

import (
	"net/http"
	"strings"
)

func (a *App) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	threadID := strings.TrimSpace(r.URL.Query().Get("thread"))
	if threadID == "" || len(threadID) > 96 {
		Error(w, http.StatusBadRequest, "Conversation not found")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,sender,body,created_at FROM chat_messages WHERE thread_id=? ORDER BY id ASC LIMIT 200`, threadID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Messages are temporarily unavailable")
		return
	}
	defer rows.Close()
	type message struct {
		ID        int64  `json:"id"`
		Sender    string `json:"sender"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
	}
	items := []message{}
	for rows.Next() {
		var item message
		if err := rows.Scan(&item.ID, &item.Sender, &item.Body, &item.CreatedAt); err != nil {
			Error(w, http.StatusInternalServerError, "Messages are temporarily unavailable")
			return
		}
		items = append(items, item)
	}
	JSON(w, http.StatusOK, Envelope{"ok": true, "messages": items})
}

func (a *App) handleChatSend(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ThreadID string `json:"thread_id"`
		Message  string `json:"message"`
	}
	if err := decodeJSON(r, &in); err != nil {
		Error(w, http.StatusBadRequest, "Please enter a message")
		return
	}
	in.ThreadID = strings.TrimSpace(in.ThreadID)
	in.Message = strings.TrimSpace(in.Message)
	if len(in.Message) < 1 || len(in.Message) > 1200 {
		Error(w, http.StatusBadRequest, "Message must be between 1 and 1200 characters")
		return
	}
	if in.ThreadID == "" {
		var err error
		in.ThreadID, err = randomURLToken(18)
		if err != nil {
			Error(w, http.StatusInternalServerError, "Could not start the conversation")
			return
		}
	}
	if len(in.ThreadID) > 96 {
		Error(w, http.StatusBadRequest, "Conversation not found")
		return
	}
	var visitorID any
	if visitor, ok := a.visitorSessionFromRequest(r); ok {
		visitorID = visitor.ID
	}
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO chat_threads(id,visitor_id,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET visitor_id=COALESCE(chat_threads.visitor_id,excluded.visitor_id),updated_at=CURRENT_TIMESTAMP`, in.ThreadID, visitorID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Message could not be sent")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO chat_messages(thread_id,sender,body) VALUES(?,'visitor',?)`, in.ThreadID, in.Message)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Message could not be sent")
		return
	}
	id, _ := res.LastInsertId()
	JSON(w, http.StatusCreated, Envelope{"ok": true, "thread_id": in.ThreadID, "id": id, "message": "Sent"})
}

func (a *App) handleAdminChats(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT t.id,
		       COALESCE(v.display_name,'Guest'),
		       COALESCE(v.email,''),
		       COALESCE(t.updated_at,t.created_at,''),
		       COALESCE((SELECT body FROM chat_messages m WHERE m.thread_id=t.id ORDER BY m.id DESC LIMIT 1),''),
		       COALESCE((SELECT sender FROM chat_messages m WHERE m.thread_id=t.id ORDER BY m.id DESC LIMIT 1),''),
		       COALESCE((SELECT created_at FROM chat_messages m WHERE m.thread_id=t.id ORDER BY m.id DESC LIMIT 1),''),
		       (SELECT COUNT(*) FROM chat_messages m WHERE m.thread_id=t.id)
		FROM chat_threads t
		LEFT JOIN visitor_accounts v ON v.id=t.visitor_id
		ORDER BY t.updated_at DESC, t.created_at DESC
		LIMIT 200`)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Store chat is unavailable right now.")
		return
	}
	defer rows.Close()
	items := []Envelope{}
	for rows.Next() {
		var id, name, email, updated, lastBody, lastSender, lastCreated string
		var count int
		if err := rows.Scan(&id, &name, &email, &updated, &lastBody, &lastSender, &lastCreated, &count); err != nil {
			Error(w, http.StatusInternalServerError, "Store chat is unavailable right now.")
			return
		}
		items = append(items, Envelope{
			"id": id, "name": name, "email": email, "updated_at": updated,
			"last_message": lastBody, "last_sender": lastSender, "last_created_at": lastCreated,
			"message_count": count, "needs_reply": lastSender == "visitor",
		})
	}
	if err := rows.Err(); err != nil {
		Error(w, http.StatusInternalServerError, "Store chat is unavailable right now.")
		return
	}
	JSON(w, http.StatusOK, Envelope{"ok": true, "threads": items})
}

func (a *App) handleAdminChatMessages(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	threadID := strings.TrimSpace(r.PathValue("id"))
	if threadID == "" || len(threadID) > 96 {
		Error(w, http.StatusBadRequest, "Conversation not found.")
		return
	}
	var exists int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM chat_threads WHERE id=?`, threadID).Scan(&exists); err != nil || exists == 0 {
		Error(w, http.StatusNotFound, "Conversation not found.")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := a.db.QueryContext(r.Context(), `SELECT id,sender,body,created_at FROM chat_messages WHERE thread_id=? ORDER BY id ASC LIMIT 400`, threadID)
		if err != nil {
			Error(w, http.StatusInternalServerError, "Conversation is unavailable right now.")
			return
		}
		defer rows.Close()
		items := []Envelope{}
		for rows.Next() {
			var id int64
			var sender, body, created string
			if err := rows.Scan(&id, &sender, &body, &created); err != nil {
				Error(w, http.StatusInternalServerError, "Conversation is unavailable right now.")
				return
			}
			items = append(items, Envelope{"id": id, "sender": sender, "body": body, "created_at": created})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			Error(w, http.StatusInternalServerError, "Conversation is unavailable right now.")
			return
		}
		// Release the single SQLite connection before reading the visitor identity.
		_ = rows.Close()
		var name, email string
		_ = a.db.QueryRowContext(r.Context(), `
			SELECT COALESCE(v.display_name,'Guest'),COALESCE(v.email,'')
			FROM chat_threads t LEFT JOIN visitor_accounts v ON v.id=t.visitor_id
			WHERE t.id=?`, threadID).Scan(&name, &email)
		JSON(w, http.StatusOK, Envelope{"ok": true, "thread_id": threadID, "name": name, "email": email, "messages": items})
		return
	}

	var in struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(r, &in); err != nil {
		Error(w, http.StatusBadRequest, "Enter a reply.")
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if len(in.Message) < 1 || len(in.Message) > 1200 {
		Error(w, http.StatusBadRequest, "Reply must be between 1 and 1200 characters.")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO chat_messages(thread_id,sender,body) VALUES(?,'staff',?)`, threadID, in.Message)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Reply could not be sent.")
		return
	}
	_, _ = a.db.ExecContext(r.Context(), `UPDATE chat_threads SET updated_at=CURRENT_TIMESTAMP WHERE id=?`, threadID)
	id, _ := res.LastInsertId()
	a.auditAdmin(staff.ID, "chat.reply", "chat:"+threadID, r)
	JSON(w, http.StatusCreated, Envelope{"ok": true, "id": id, "message": "Reply sent."})
}
