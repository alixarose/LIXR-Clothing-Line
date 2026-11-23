package internals

import (
	"net/http"
	"path/filepath"
)

func (a *App) routes(mux *http.ServeMux) {
	// Public API.
	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("GET /api/store", a.handleStore)
	mux.HandleFunc("GET /api/products", a.handleProducts)
	mux.HandleFunc("GET /api/products/{id}/image", a.handleProductImage)
	mux.HandleFunc("GET /api/ratings", a.handleRatings)
	mux.HandleFunc("POST /api/ratings", a.handleCreateRating)
	mux.HandleFunc("GET /api/visitor/session", a.handleVisitorSession)
	mux.HandleFunc("GET /api/auth/google/start", a.handleGoogleStart)
	mux.HandleFunc("GET /api/auth/google/callback", a.handleGoogleCallback)
	mux.HandleFunc("POST /api/visitor/logout", a.handleVisitorLogout)
	mux.HandleFunc("GET /api/chat/messages", a.handleChatMessages)
	mux.HandleFunc("POST /api/chat/messages", a.handleChatSend)
	mux.HandleFunc("POST /api/contact", a.handleContact)
	mux.HandleFunc("POST /api/newsletter", a.handleNewsletter)
	mux.HandleFunc("GET /api/newsletter/unsubscribe", a.handleNewsletterUnsubscribe)
	mux.HandleFunc("POST /api/analytics/visit", a.handleTrackVisit)

	// Staff session API.
	mux.HandleFunc("GET /api/auth/session", a.handleAuthSession)
	mux.HandleFunc("POST /api/auth/login", a.handleAuthLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleAuthLogout)

	// Connected Admin API.
	mux.HandleFunc("GET /api/admin/dashboard", a.handleAdminDashboard)
	mux.HandleFunc("GET /api/admin/inventory", a.handleAdminInventory)
	mux.HandleFunc("POST /api/admin/products", a.handleAdminCreateProduct)
	mux.HandleFunc("PUT /api/admin/products/{id}", a.handleAdminUpdateProduct)
	mux.HandleFunc("DELETE /api/admin/products/{id}", a.handleAdminDeleteProduct)
	mux.HandleFunc("GET /api/admin/products/{id}/stock", a.handleAdminProductStock)
	mux.HandleFunc("PUT /api/admin/products/{id}/stock", a.handleAdminProductStock)
	mux.HandleFunc("GET /api/admin/staff", a.handleAdminStaff)
	mux.HandleFunc("POST /api/admin/staff", a.handleAdminCreateStaff)
	mux.HandleFunc("PATCH /api/admin/staff/{id}", a.handleAdminUpdateStaff)
	mux.HandleFunc("GET /api/admin/contacts", a.handleAdminContacts)
	mux.HandleFunc("PATCH /api/admin/contacts/{id}", a.handleAdminContactStatus)
	mux.HandleFunc("GET /api/admin/chats", a.handleAdminChats)
	mux.HandleFunc("GET /api/admin/chats/{id}/messages", a.handleAdminChatMessages)
	mux.HandleFunc("POST /api/admin/chats/{id}/messages", a.handleAdminChatMessages)
	mux.HandleFunc("GET /api/admin/ratings", a.handleAdminRatings)
	mux.HandleFunc("PATCH /api/admin/ratings/{id}", a.handleAdminRatingVisibility)
	mux.HandleFunc("GET /api/admin/system", a.handleAdminSystem)
	mux.HandleFunc("GET /api/admin/sales", a.handleAdminSales)
	mux.HandleFunc("GET /api/admin/sales/{id}/receipt", a.handleAdminReceipt)
	mux.HandleFunc("GET /api/admin/database", a.handleAdminDatabase)
	mux.HandleFunc("POST /api/admin/database/query", a.handleAdminDatabaseQuery)
	mux.HandleFunc("POST /api/admin/database/reset", a.handleAdminDatabaseReset)

	clientRoot := filepath.Join("..", "client")
	mux.Handle("GET /styles/", http.StripPrefix("/styles/", http.FileServer(http.Dir(filepath.Join(clientRoot, "styles")))))
	mux.Handle("GET /scripts/", http.StripPrefix("/scripts/", http.FileServer(http.Dir(filepath.Join(clientRoot, "scripts")))))
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(clientRoot, "assets")))))

	mux.HandleFunc("GET /products", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(clientRoot, "structure", "products.html"))
	})
	mux.HandleFunc("GET /products.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/products", http.StatusMovedPermanently)
	})

	mux.HandleFunc("GET /access", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(clientRoot, "structure", "access.html"))
	})
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/access", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("GET /access.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/access", http.StatusMovedPermanently)
	})

	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		staff, ok := a.sessionFromRequest(r)
		if !ok {
			http.Redirect(w, r, "/access?next=/admin", http.StatusSeeOther)
			return
		}
		if staff.Role == "cashier" {
			http.Redirect(w, r, "/pos", http.StatusSeeOther)
			return
		}
		http.ServeFile(w, r, filepath.Join(clientRoot, "structure", "admin.html"))
	})
	mux.HandleFunc("GET /admin.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /pos", func(w http.ResponseWriter, r *http.Request) {
		staff, ok := a.sessionFromRequest(r)
		if !ok {
			http.Redirect(w, r, "/access?next=/pos", http.StatusSeeOther)
			return
		}
		if staff.Role != "cashier" && staff.Role != "admin" && staff.Role != "developer" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, filepath.Join(clientRoot, "structure", "pos.html"))
	})

	mux.HandleFunc("GET /pos.html", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/pos", http.StatusMovedPermanently) })

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(clientRoot, "structure", "index.html"))
	})
}
