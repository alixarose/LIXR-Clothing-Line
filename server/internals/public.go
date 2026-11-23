package internals

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ProductSize struct {
	Label    string `json:"label"`
	Quantity int    `json:"quantity"`
}

type Product struct {
	ID             int64         `json:"id"`
	Slug           string        `json:"slug"`
	SKU            string        `json:"sku"`
	Barcode        string        `json:"barcode"`
	Name           string        `json:"name"`
	Subtitle       string        `json:"subtitle"`
	Category       string        `json:"category"`
	Collection     string        `json:"collection"`
	Description    string        `json:"description"`
	Color          string        `json:"color"`
	Size           string        `json:"size"`
	Sizes          []ProductSize `json:"sizes"`
	HasSizes       bool          `json:"has_sizes"`
	CostPrice      float64       `json:"cost_price,omitempty"`
	RetailPrice    float64       `json:"retail_price"`
	SalePrice      float64       `json:"sale_price"`
	Price          float64       `json:"price"`
	ComparePrice   *float64      `json:"compare_price,omitempty"`
	ImageURL       string        `json:"image_url"`
	ImageSourceURL string        `json:"image_source_url,omitempty"`
	ImageAlt       string        `json:"image_alt"`
	Badge          string        `json:"badge"`
	Stock          int           `json:"stock"`
	LowThreshold   int           `json:"low_stock_threshold"`
	Featured       bool          `json:"featured"`
	Active         bool          `json:"active"`
	Availability   string        `json:"availability"`
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if err := a.db.PingContext(r.Context()); err != nil {
		Error(w, http.StatusServiceUnavailable, "store service unavailable")
		return
	}
	JSON(w, http.StatusOK, Envelope{
		"ok": true, "app": a.cfg.AppName, "database": "SQLite",
		"latency_ms":       time.Since(start).Milliseconds(),
		"time":             time.Now().UTC().Format(time.RFC3339),
		"oauth_configured": a.cfg.GoogleOAuthEnabled && a.cfg.GoogleClientID != "" && a.cfg.GoogleClientSecret != "",
		"smtp_configured":  a.cfg.SMTPEnabled && a.cfg.SMTPHost != "" && a.cfg.SMTPUsername != "" && a.cfg.SMTPPassword != "",
	})
}

func (a *App) handleStore(w http.ResponseWriter, r *http.Request) {
	var store struct {
		StoreName    string `json:"store_name"`
		Tagline      string `json:"tagline"`
		Announcement string `json:"announcement"`
		Address      string `json:"address"`
		Hours        string `json:"hours"`
		Phone        string `json:"phone"`
		Email        string `json:"email"`
	}
	err := a.db.QueryRowContext(r.Context(), `SELECT store_name,tagline,announcement,address,hours,phone,email FROM store_settings WHERE id=1`).
		Scan(&store.StoreName, &store.Tagline, &store.Announcement, &store.Address, &store.Hours, &store.Phone, &store.Email)
	if err != nil {
		Error(w, http.StatusInternalServerError, "store details unavailable")
		return
	}
	JSON(w, http.StatusOK, Envelope{"ok": true, "store": store})
}

func (a *App) handleProducts(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id,slug,sku,barcode,name,subtitle,category,collection_name,description,color,size,
		cost_price,retail_price,sale_price,price,compare_price,
		CASE WHEN image_data IS NOT NULL AND length(image_data)>0 THEN '/api/products/' || id || '/image' ELSE image_url END,
		COALESCE(image_source_url,''),image_alt,badge,stock,low_stock_threshold,featured,active
		FROM products WHERE active=1`
	args := []any{}
	if r.URL.Query().Get("featured") == "1" {
		query += ` AND featured=1`
	}
	if collection := strings.TrimSpace(r.URL.Query().Get("collection")); collection != "" {
		query += ` AND collection_name=?`
		args = append(args, collection)
	}
	query += ` ORDER BY featured DESC,id ASC LIMIT 120`
	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		Error(w, http.StatusInternalServerError, "products unavailable")
		return
	}
	items := []Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			_ = rows.Close()
			Error(w, http.StatusInternalServerError, "products unavailable")
			return
		}
		p.CostPrice = 0
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		Error(w, http.StatusInternalServerError, "products unavailable")
		return
	}
	_ = rows.Close()
	if err := a.attachProductSizes(r.Context(), items); err != nil {
		Error(w, http.StatusInternalServerError, "products unavailable")
		return
	}
	JSON(w, http.StatusOK, Envelope{"ok": true, "products": items})
}

type rowScanner interface{ Scan(dest ...any) error }

func scanProduct(row rowScanner) (Product, error) {
	var p Product
	var compare sql.NullFloat64
	var featured, active int
	err := row.Scan(&p.ID, &p.Slug, &p.SKU, &p.Barcode, &p.Name, &p.Subtitle, &p.Category, &p.Collection, &p.Description, &p.Color, &p.Size,
		&p.CostPrice, &p.RetailPrice, &p.SalePrice, &p.Price, &compare, &p.ImageURL, &p.ImageSourceURL, &p.ImageAlt, &p.Badge, &p.Stock, &p.LowThreshold, &featured, &active)
	if err != nil {
		return p, err
	}
	if compare.Valid {
		p.ComparePrice = &compare.Float64
	}
	p.Featured = featured == 1
	p.Active = active == 1
	if p.RetailPrice <= 0 {
		p.RetailPrice = p.Price
	}
	if p.SalePrice <= 0 {
		p.SalePrice = p.RetailPrice
	}
	p.Price = p.SalePrice
	switch {
	case p.Stock <= 0:
		p.Availability = "Sold out"
	case p.Stock <= p.LowThreshold:
		p.Availability = "Low stock"
	default:
		p.Availability = "Available in store"
	}
	return p, nil
}

func isDirectStockLabel(label string) bool {
	value := strings.ToLower(strings.TrimSpace(label))
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.Join(strings.Fields(value), " ")
	return value == "" || value == "one size" || value == "onesize" || value == "n/a" || value == "na" || value == "default"
}

func (a *App) attachProductSizes(ctx context.Context, products []Product) error {
	if len(products) == 0 {
		return nil
	}
	rows, err := a.db.QueryContext(ctx, `SELECT product_id,size_label,quantity FROM product_sizes ORDER BY product_id,id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	byProduct := map[int64][]ProductSize{}
	for rows.Next() {
		var productID int64
		var size ProductSize
		if err := rows.Scan(&productID, &size.Label, &size.Quantity); err != nil {
			return err
		}
		byProduct[productID] = append(byProduct[productID], size)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range products {
		sizes := byProduct[products[i].ID]
		if len(sizes) == 0 {
			if !isDirectStockLabel(products[i].Size) {
				products[i].HasSizes = true
				products[i].Sizes = []ProductSize{{Label: products[i].Size, Quantity: products[i].Stock}}
			}
			continue
		}
		if len(sizes) == 1 && isDirectStockLabel(sizes[0].Label) {
			products[i].HasSizes = false
			products[i].Sizes = []ProductSize{}
			products[i].Size = ""
			continue
		}
		products[i].HasSizes = true
		products[i].Sizes = sizes
	}
	return nil
}

func (a *App) handleRatings(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1, 1, 100000)
	limit := queryInt(r, "limit", 6, 1, 12)
	offset := (page - 1) * limit
	var total int
	var avg float64
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*),COALESCE(AVG(stars),0) FROM ratings WHERE visible=1`).Scan(&total, &avg)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,display_name,stars,comment,created_at FROM ratings WHERE visible=1 ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		Error(w, 500, "reviews unavailable")
		return
	}
	defer rows.Close()
	type rating struct {
		ID          int64  `json:"id"`
		DisplayName string `json:"display_name"`
		Stars       int    `json:"stars"`
		Comment     string `json:"comment"`
		CreatedAt   string `json:"created_at"`
	}
	items := []rating{}
	for rows.Next() {
		var it rating
		if err := rows.Scan(&it.ID, &it.DisplayName, &it.Stars, &it.Comment, &it.CreatedAt); err != nil {
			Error(w, 500, "reviews unavailable")
			return
		}
		items = append(items, it)
	}
	pages := 0
	if total > 0 {
		pages = (total + limit - 1) / limit
	}
	JSON(w, 200, Envelope{"ok": true, "average": avg, "total": total, "page": page, "pages": pages, "ratings": items})
}

func (a *App) handleContact(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Email, Subject, Message string }
	if err := decodeJSON(r, &in); err != nil {
		Error(w, 400, "Please check the form and try again.")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Subject = strings.TrimSpace(in.Subject)
	in.Message = strings.TrimSpace(in.Message)
	if len(in.Name) < 2 || !strings.Contains(in.Email, "@") || len(in.Subject) < 2 || len(in.Message) < 10 {
		Error(w, 400, "Please complete every contact field.")
		return
	}
	if len(in.Name) > 120 || len(in.Email) > 254 || len(in.Subject) > 180 || len(in.Message) > 4000 {
		Error(w, 400, "Your message is a little too long. Please shorten it and try again.")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO contact_messages(name,email,subject,message) VALUES(?,?,?,?)`, in.Name, in.Email, in.Subject, in.Message)
	if err != nil {
		Error(w, 500, "Your message could not be sent right now.")
		return
	}
	id, _ := res.LastInsertId()
	JSON(w, 201, Envelope{"ok": true, "message": "Message received. Lixr will get back to you soon.", "id": id})
}

func (a *App) handleNewsletter(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &in); err != nil {
		Error(w, 400, "Please enter your email again.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if len(email) > 254 || !strings.Contains(email, "@") {
		Error(w, 400, "Enter a valid email address.")
		return
	}
	var existingID int64
	var existingToken, createdAt string
	var active int
	err := a.db.QueryRowContext(r.Context(), `SELECT id,COALESCE(unsubscribe_token,''),active,created_at FROM newsletter_subscribers WHERE email=?`, email).Scan(&existingID, &existingToken, &active, &createdAt)
	if err == nil {
		if active == 1 {
			JSON(w, 200, Envelope{"ok": true, "already_subscribed": true, "subscribed_at": createdAt, "message": "You already joined the Lixr list on " + createdAt + "."})
			return
		}
		if existingToken == "" {
			existingToken, _ = randomURLToken(24)
		}
		_, err = a.db.ExecContext(r.Context(), `UPDATE newsletter_subscribers SET active=1,unsubscribe_token=?,unsubscribed_at=NULL,created_at=CURRENT_TIMESTAMP WHERE id=?`, existingToken, existingID)
		if err != nil {
			Error(w, 500, "Your subscription could not be updated right now.")
			return
		}
		_ = a.sendNewsletterWelcome(r, email, existingToken)
		JSON(w, 200, Envelope{"ok": true, "message": "Welcome back to the Lixr list."})
		return
	}
	if err != sql.ErrNoRows {
		Error(w, 500, "Your subscription could not be saved right now.")
		return
	}
	token, err := randomURLToken(24)
	if err != nil {
		Error(w, 500, "Your subscription could not be saved right now.")
		return
	}
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO newsletter_subscribers(email,unsubscribe_token,active) VALUES(?,?,1)`, email, token)
	if err != nil {
		Error(w, 500, "Your subscription could not be saved right now.")
		return
	}
	_ = a.sendNewsletterWelcome(r, email, token)
	JSON(w, 201, Envelope{"ok": true, "message": "You’re on the Lixr list. Check your inbox for a welcome note."})
}

func (a *App) handleTrackVisit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Session string `json:"session"`
		Path    string `json:"path"`
	}
	if err := decodeJSON(r, &in); err != nil {
		Error(w, 400, "invalid visit")
		return
	}
	in.Session = strings.TrimSpace(in.Session)
	in.Path = strings.TrimSpace(in.Path)
	if len(in.Session) < 8 || len(in.Session) > 120 {
		Error(w, 400, "invalid visit")
		return
	}
	if in.Path == "" || len(in.Path) > 180 || !strings.HasPrefix(in.Path, "/") {
		in.Path = "/"
	}
	var visitor any
	if v, ok := a.visitorSessionFromRequest(r); ok {
		visitor = v.ID
	}
	// One row per session/path/hour avoids refreshes inflating the dashboard.
	var existing int
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM page_visits WHERE session_key=? AND path=? AND created_at>=datetime('now','-1 hour')`, in.Session, in.Path).Scan(&existing)
	if existing == 0 {
		_, _ = a.db.ExecContext(r.Context(), `INSERT INTO page_visits(session_key,visitor_id,path) VALUES(?,?,?)`, in.Session, visitor, in.Path)
	}
	JSON(w, 200, Envelope{"ok": true})
}

func queryInt(r *http.Request, key string, fallback, min, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil || value < min || value > max {
		return fallback
	}
	return value
}
