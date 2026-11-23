package internals

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	count := func(query string, args ...any) int {
		var v int
		_ = a.db.QueryRowContext(r.Context(), query, args...).Scan(&v)
		return v
	}
	sum := func(query string, args ...any) float64 {
		var v float64
		_ = a.db.QueryRowContext(r.Context(), query, args...).Scan(&v)
		return v
	}
	avgRating := sum(`SELECT COALESCE(AVG(stars),0) FROM ratings WHERE visible=1`)
	inventoryValue := sum(`SELECT COALESCE(SUM(cost_price*stock),0) FROM products WHERE active=1`)
	revenue := sum(`SELECT COALESCE(SUM(total),0) FROM sales WHERE status='completed'`)
	costOfSales := sum(`SELECT COALESCE(SUM(cost_total),0) FROM sales WHERE status='completed'`)
	profit := revenue - costOfSales
	margin := 0.0
	if revenue > 0 {
		margin = profit / revenue * 100
	}

	labels := make([]string, 0, 14)
	visitors := make([]int, 0, 14)
	dailySales := make([]float64, 0, 14)
	dailyProfit := make([]float64, 0, 14)
	today := time.Now()
	for i := 13; i >= 0; i-- {
		day := today.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		labels = append(labels, day.Format("Jan 02"))
		visitors = append(visitors, count(`SELECT COUNT(DISTINCT session_key) FROM page_visits WHERE date(created_at,'localtime')=?`, key))
		dailySales = append(dailySales, sum(`SELECT COALESCE(SUM(total),0) FROM sales WHERE status='completed' AND date(created_at,'localtime')=?`, key))
		dailyProfit = append(dailyProfit, sum(`SELECT COALESCE(SUM(total-cost_total),0) FROM sales WHERE status='completed' AND date(created_at,'localtime')=?`, key))
	}

	recentSales := []Envelope{}
	if rows, err := a.db.QueryContext(r.Context(), `SELECT id,COALESCE(receipt_no,''),customer_name,total,status,created_at FROM sales ORDER BY id DESC LIMIT 5`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var receipt, customer, status, created string
			var totalValue float64
			if rows.Scan(&id, &receipt, &customer, &totalValue, &status, &created) == nil {
				recentSales = append(recentSales, Envelope{"id": id, "receipt_no": receipt, "customer": customer, "total": totalValue, "status": status, "created_at": created})
			}
		}
	}
	stockWatch := []Envelope{}
	if rows, err := a.db.QueryContext(r.Context(), `SELECT id,name,sku,stock,low_stock_threshold FROM products WHERE active=1 AND stock<=low_stock_threshold ORDER BY stock ASC,name ASC LIMIT 6`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var name, sku string
			var stock, threshold int
			if rows.Scan(&id, &name, &sku, &stock, &threshold) == nil {
				stockWatch = append(stockWatch, Envelope{"id": id, "name": name, "sku": sku, "stock": stock, "threshold": threshold})
			}
		}
	}

	categoryLabels := []string{}
	categoryStock := []int{}
	if rows, err := a.db.QueryContext(r.Context(), `SELECT category,COALESCE(SUM(stock),0) FROM products WHERE active=1 GROUP BY category ORDER BY SUM(stock) DESC,category LIMIT 8`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var label string
			var stock int
			if rows.Scan(&label, &stock) == nil {
				categoryLabels = append(categoryLabels, label)
				categoryStock = append(categoryStock, stock)
			}
		}
	}

	JSON(w, http.StatusOK, Envelope{"ok": true, "dashboard": Envelope{
		"products":             count(`SELECT COUNT(*) FROM products WHERE active=1`),
		"low_stock":            count(`SELECT COUNT(*) FROM products WHERE active=1 AND stock>0 AND stock<=low_stock_threshold`),
		"out_of_stock":         count(`SELECT COUNT(*) FROM products WHERE active=1 AND stock<=0`),
		"messages":             count(`SELECT COUNT(*) FROM contact_messages WHERE status='new'`) + count(`SELECT COUNT(*) FROM chat_threads t WHERE (SELECT sender FROM chat_messages m WHERE m.thread_id=t.id ORDER BY m.id DESC LIMIT 1)='visitor'`),
		"contact_new":          count(`SELECT COUNT(*) FROM contact_messages WHERE status='new'`),
		"chat_needs_reply":     count(`SELECT COUNT(*) FROM chat_threads t WHERE (SELECT sender FROM chat_messages m WHERE m.thread_id=t.id ORDER BY m.id DESC LIMIT 1)='visitor'`),
		"ratings":              count(`SELECT COUNT(*) FROM ratings WHERE visible=1`),
		"staff":                count(`SELECT COUNT(*) FROM staff_accounts WHERE active=1`),
		"average_rating":       avgRating,
		"inventory_cost_value": inventoryValue,
		"visitors_today":       count(`SELECT COUNT(DISTINCT session_key) FROM page_visits WHERE date(created_at,'localtime')=date('now','localtime')`),
		"visitors_total":       count(`SELECT COUNT(DISTINCT session_key) FROM page_visits`),
		"sales_count":          count(`SELECT COUNT(*) FROM sales WHERE status='completed'`),
		"sales_today":          count(`SELECT COUNT(*) FROM sales WHERE status='completed' AND date(created_at,'localtime')=date('now','localtime')`),
		"revenue":              revenue,
		"profit":               profit,
		"margin_percent":       margin,
		"chart_labels":         labels,
		"chart_visitors":       visitors,
		"chart_sales":          dailySales,
		"chart_profit":         dailyProfit,
		"category_labels":      categoryLabels,
		"category_stock":       categoryStock,
		"recent_sales":         recentSales,
		"stock_watch":          stockWatch,
	}})
}

func (a *App) handleAdminInventory(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	query := `SELECT id,slug,sku,barcode,name,subtitle,category,collection_name,description,color,size,
        cost_price,retail_price,sale_price,price,compare_price,
        CASE WHEN image_data IS NOT NULL AND length(image_data)>0 THEN '/api/products/' || id || '/image' ELSE image_url END,
        COALESCE(image_source_url,''),image_alt,badge,stock,low_stock_threshold,featured,active FROM products`
	args := []any{}
	if q != "" {
		like := "%" + q + "%"
		query += ` WHERE name LIKE ? OR sku LIKE ? OR barcode LIKE ? OR category LIKE ? OR collection_name LIKE ? OR color LIKE ? OR size LIKE ?`
		args = append(args, like, like, like, like, like, like, like)
	}
	query += ` ORDER BY active DESC,featured DESC,name ASC`
	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		Error(w, 500, "Inventory is unavailable right now.")
		return
	}
	items := []Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			_ = rows.Close()
			Error(w, 500, "Inventory is unavailable right now.")
			return
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		Error(w, 500, "Inventory is unavailable right now.")
		return
	}
	_ = rows.Close()
	if err := a.attachProductSizes(r.Context(), items); err != nil {
		Error(w, 500, "Inventory is unavailable right now.")
		return
	}
	JSON(w, 200, Envelope{"ok": true, "products": items})
}

type productInput struct {
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	SKU          string  `json:"sku"`
	Barcode      string  `json:"barcode"`
	Subtitle     string  `json:"subtitle"`
	Category     string  `json:"category"`
	Collection   string  `json:"collection"`
	Description  string  `json:"description"`
	Color        string  `json:"color"`
	Size         string  `json:"size"`
	CostPrice    float64 `json:"cost_price"`
	RetailPrice  float64 `json:"retail_price"`
	SalePrice    float64 `json:"sale_price"`
	ImageURL     string  `json:"image_url"`
	ImageData    string  `json:"image_data,omitempty"`
	ImageMime    string  `json:"image_mime,omitempty"`
	ImageAlt     string  `json:"image_alt"`
	Badge        string  `json:"badge"`
	Stock        int     `json:"stock"`
	LowThreshold int     `json:"low_stock_threshold"`
	Featured     bool    `json:"featured"`
	Active       bool    `json:"active"`
}

func normalizeProductInput(in *productInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = slugify(in.Slug)
	if in.Slug == "" {
		in.Slug = slugify(in.Name)
	}
	in.SKU = strings.ToUpper(strings.TrimSpace(in.SKU))
	in.Barcode = strings.TrimSpace(in.Barcode)
	in.Subtitle = strings.TrimSpace(in.Subtitle)
	in.Category = strings.TrimSpace(in.Category)
	in.Collection = strings.TrimSpace(in.Collection)
	in.Description = strings.TrimSpace(in.Description)
	in.Color = strings.TrimSpace(in.Color)
	in.Size = strings.TrimSpace(in.Size)
	in.ImageURL = strings.TrimSpace(in.ImageURL)
	in.ImageAlt = strings.TrimSpace(in.ImageAlt)
	in.Badge = strings.TrimSpace(in.Badge)
	if len(in.Name) < 2 || len(in.Name) > 160 {
		return errors.New("product name must be 2–160 characters")
	}
	if in.Slug == "" || len(in.Slug) > 180 {
		return errors.New("product slug is invalid")
	}
	if len(in.SKU) < 2 || len(in.SKU) > 120 {
		return errors.New("SKU is required")
	}
	if len(in.Category) < 2 || len(in.Category) > 100 {
		return errors.New("category is required")
	}
	if in.CostPrice < 0 || in.RetailPrice < 0 || in.SalePrice < 0 {
		return errors.New("prices cannot be negative")
	}
	if in.RetailPrice <= 0 {
		return errors.New("retail price must be greater than zero")
	}
	if in.SalePrice <= 0 {
		in.SalePrice = in.RetailPrice
	}
	if in.Stock < 0 {
		return errors.New("stock cannot be negative")
	}
	if in.LowThreshold < 0 {
		in.LowThreshold = 0
	}
	if in.LowThreshold > 100000 {
		in.LowThreshold = 100000
	}
	if in.ImageURL == "" && strings.TrimSpace(in.ImageData) == "" {
		return errors.New("add an image URL or upload an image")
	}
	if in.ImageURL != "" && !strings.HasPrefix(in.ImageURL, "http://") && !strings.HasPrefix(in.ImageURL, "https://") && !strings.HasPrefix(in.ImageURL, "/api/products/") {
		return errors.New("image URL must start with http:// or https://")
	}
	return nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (a *App) productImageForInput(r *http.Request, in productInput, existingID int64) ([]byte, string, string, error) {
	sourceURL := strings.TrimSpace(in.ImageURL)
	if strings.TrimSpace(in.ImageData) != "" {
		raw, err := decodeProductUpload(in.ImageData)
		if err != nil {
			return nil, "", "", err
		}
		data, mimeType, err := optimizeProductImage(raw, in.ImageMime)
		return data, mimeType, sourceURL, err
	}
	if existingID > 0 && strings.HasPrefix(sourceURL, "/api/products/") {
		return nil, "", "", nil
	}
	raw, hinted, err := fetchProductImage(r.Context(), sourceURL)
	if err != nil {
		return nil, "", "", err
	}
	data, mimeType, err := optimizeProductImage(raw, hinted)
	return data, mimeType, sourceURL, err
}

func (a *App) handleAdminCreateProduct(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var in productInput
	if err := decodeJSON(r, &in); err != nil {
		Error(w, 400, "Please check the product details and try again.")
		return
	}
	if err := normalizeProductInput(&in); err != nil {
		Error(w, 400, err.Error())
		return
	}
	data, mimeType, sourceURL, err := a.productImageForInput(r, in, 0)
	if err != nil {
		Error(w, 400, err.Error())
		return
	}
	featured, active := 0, 0
	if in.Featured {
		featured = 1
	}
	if in.Active {
		active = 1
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		Error(w, 500, "Product could not be added.")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO products(slug,sku,barcode,name,subtitle,category,collection_name,description,color,size,cost_price,retail_price,sale_price,price,image_url,image_source_url,image_data,image_mime,image_alt,badge,stock,low_stock_threshold,featured,active,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
		in.Slug, in.SKU, in.Barcode, in.Name, in.Subtitle, in.Category, in.Collection, in.Description, in.Color, in.Size, in.CostPrice, in.RetailPrice, in.SalePrice, in.SalePrice, sourceURL, sourceURL, data, mimeType, in.ImageAlt, in.Badge, 0, in.LowThreshold, featured, active)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			Error(w, 409, "Slug or SKU already exists.")
			return
		}
		Error(w, 500, "Product could not be added.")
		return
	}
	id, _ := res.LastInsertId()
	if in.Stock > 0 {
		label := in.Size
		if label == "" {
			label = "One size"
		}
		_, _ = tx.ExecContext(r.Context(), `INSERT INTO product_sizes(product_id,size_label,quantity) VALUES(?,?,?)`, id, label, in.Stock)
		_, _ = tx.ExecContext(r.Context(), `UPDATE products SET stock=? WHERE id=?`, in.Stock, id)
	}
	if err = tx.Commit(); err != nil {
		Error(w, 500, "Product could not be added.")
		return
	}
	a.auditAdmin(staff.ID, "product.created", fmt.Sprintf("product:%d", id), r)
	JSON(w, 201, Envelope{"ok": true, "id": id, "message": "Product added."})
}

func (a *App) handleAdminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		Error(w, 400, "Invalid product.")
		return
	}
	var in productInput
	if err := decodeJSON(r, &in); err != nil {
		Error(w, 400, "Please check the product details and try again.")
		return
	}
	if strings.TrimSpace(in.ImageURL) == "" && strings.TrimSpace(in.ImageData) == "" {
		var imageCount int
		if err := a.db.QueryRowContext(r.Context(), `SELECT CASE WHEN image_data IS NOT NULL AND length(image_data)>0 THEN 1 WHEN COALESCE(image_source_url,image_url,'')<>'' THEN 1 ELSE 0 END FROM products WHERE id=?`, id).Scan(&imageCount); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				Error(w, 404, "Product not found.")
			} else {
				Error(w, 500, "Product could not be updated.")
			}
			return
		}
		if imageCount > 0 {
			in.ImageURL = fmt.Sprintf("/api/products/%d/image", id)
		}
	}
	if err := normalizeProductInput(&in); err != nil {
		Error(w, 400, err.Error())
		return
	}
	featured, active := 0, 0
	if in.Featured {
		featured = 1
	}
	if in.Active {
		active = 1
	}
	var data []byte
	var mimeType, sourceURL string
	if strings.TrimSpace(in.ImageData) != "" || (in.ImageURL != "" && !strings.HasPrefix(in.ImageURL, "/api/products/")) {
		data, mimeType, sourceURL, err = a.productImageForInput(r, in, id)
		if err != nil {
			Error(w, 400, err.Error())
			return
		}
	}
	var res sql.Result
	if len(data) > 0 {
		res, err = a.db.ExecContext(r.Context(), `UPDATE products SET slug=?,sku=?,barcode=?,name=?,subtitle=?,category=?,collection_name=?,description=?,color=?,size=?,cost_price=?,retail_price=?,sale_price=?,price=?,image_url=?,image_source_url=?,image_data=?,image_mime=?,image_alt=?,badge=?,low_stock_threshold=?,featured=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			in.Slug, in.SKU, in.Barcode, in.Name, in.Subtitle, in.Category, in.Collection, in.Description, in.Color, in.Size, in.CostPrice, in.RetailPrice, in.SalePrice, in.SalePrice, sourceURL, sourceURL, data, mimeType, in.ImageAlt, in.Badge, in.LowThreshold, featured, active, id)
	} else {
		res, err = a.db.ExecContext(r.Context(), `UPDATE products SET slug=?,sku=?,barcode=?,name=?,subtitle=?,category=?,collection_name=?,description=?,color=?,size=?,cost_price=?,retail_price=?,sale_price=?,price=?,image_alt=?,badge=?,low_stock_threshold=?,featured=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			in.Slug, in.SKU, in.Barcode, in.Name, in.Subtitle, in.Category, in.Collection, in.Description, in.Color, in.Size, in.CostPrice, in.RetailPrice, in.SalePrice, in.SalePrice, in.ImageAlt, in.Badge, in.LowThreshold, featured, active, id)
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			Error(w, 409, "Slug or SKU already exists.")
			return
		}
		Error(w, 500, "Product could not be updated.")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		Error(w, 404, "Product not found.")
		return
	}
	a.auditAdmin(staff.ID, "product.updated", fmt.Sprintf("product:%d", id), r)
	JSON(w, 200, Envelope{"ok": true, "message": "Product updated."})
}

func (a *App) handleAdminStaff(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,username,COALESCE(email,''),display_name,role,active,protected,COALESCE(last_login_at,''),created_at FROM staff_accounts ORDER BY protected DESC, role, username`)
	if err != nil {
		Error(w, 500, "staff unavailable")
		return
	}
	defer rows.Close()
	type row struct {
		ID          int64  `json:"id"`
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Active      bool   `json:"active"`
		Protected   bool   `json:"protected"`
		LastLogin   string `json:"last_login_at"`
		CreatedAt   string `json:"created_at"`
	}
	items := []row{}
	for rows.Next() {
		var x row
		var active, protected int
		if err := rows.Scan(&x.ID, &x.Username, &x.Email, &x.DisplayName, &x.Role, &active, &protected, &x.LastLogin, &x.CreatedAt); err != nil {
			Error(w, 500, "staff unavailable")
			return
		}
		x.Active = active == 1
		x.Protected = protected == 1
		items = append(items, x)
	}
	JSON(w, 200, Envelope{"ok": true, "staff": items})
}

type staffInput struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Password    string `json:"password"`
	Active      *bool  `json:"active,omitempty"`
}

func validateRole(role string) bool {
	return role == "cashier" || role == "admin" || role == "developer"
}

func (a *App) handleAdminCreateStaff(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var in staffInput
	if err := decodeJSON(r, &in); err != nil {
		Error(w, 400, "invalid staff payload")
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	if len(in.Username) < 3 || len(in.Username) > 80 || strings.ContainsAny(in.Username, " @/\\") {
		Error(w, 400, "username must be 3–80 characters without spaces")
		return
	}
	if in.Email != "" && !strings.Contains(in.Email, "@") {
		Error(w, 400, "email is invalid")
		return
	}
	if len(in.DisplayName) < 2 || len(in.DisplayName) > 160 {
		Error(w, 400, "display name is required")
		return
	}
	if !validateRole(in.Role) {
		Error(w, 400, "role must be cashier, admin, or developer")
		return
	}
	if actor.Role == "admin" && in.Role == "developer" {
		Error(w, 403, "admins cannot create developer accounts")
		return
	}
	if len(in.Password) < 8 || len(in.Password) > 256 {
		Error(w, 400, "password must be at least 8 characters")
		return
	}
	salt, hash, err := createPasswordRecord(in.Password)
	if err != nil {
		Error(w, 500, "could not create staff credential")
		return
	}
	protected := 0
	if in.Role == "developer" {
		protected = 1
	}
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO staff_accounts(username,email,display_name,role,password_salt,password_hash,password_iterations,active,protected) VALUES(?,?,?,?,?,?,?,?,?)`,
		in.Username, nullIfEmpty(in.Email), in.DisplayName, in.Role, salt, hash, passwordIterations, 1, protected)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			Error(w, 409, "username or email already exists")
			return
		}
		Error(w, 500, "could not create staff account")
		return
	}
	a.auditAdmin(actor.ID, "staff.created", "staff:"+in.Username, r)
	JSON(w, 201, Envelope{"ok": true, "message": "Staff account created."})
}

func (a *App) handleAdminUpdateStaff(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := pathID(r.URL.Path, "/api/admin/staff/")
	if err != nil {
		Error(w, 400, "invalid staff id")
		return
	}
	var current struct {
		ID                                 int64
		Username, Email, DisplayName, Role string
		Active, Protected                  int
	}
	err = a.db.QueryRowContext(r.Context(), `SELECT id,username,COALESCE(email,''),display_name,role,active,protected FROM staff_accounts WHERE id=?`, id).
		Scan(&current.ID, &current.Username, &current.Email, &current.DisplayName, &current.Role, &current.Active, &current.Protected)
	if errors.Is(err, sql.ErrNoRows) {
		Error(w, 404, "staff account not found")
		return
	}
	if err != nil {
		Error(w, 500, "staff account unavailable")
		return
	}
	if actor.Role == "admin" && (current.Role == "developer" || current.Protected == 1) {
		Error(w, 403, "developer accounts cannot be modified by admin accounts")
		return
	}

	var in staffInput
	if err := decodeJSON(r, &in); err != nil {
		Error(w, 400, "invalid staff payload")
		return
	}
	if strings.TrimSpace(in.DisplayName) != "" {
		current.DisplayName = strings.TrimSpace(in.DisplayName)
	}
	if strings.TrimSpace(in.Email) != "" {
		current.Email = strings.ToLower(strings.TrimSpace(in.Email))
	}
	if strings.TrimSpace(in.Role) != "" {
		role := strings.ToLower(strings.TrimSpace(in.Role))
		if !validateRole(role) {
			Error(w, 400, "invalid role")
			return
		}
		if actor.Role == "admin" && role == "developer" {
			Error(w, 403, "admins cannot promote accounts to developer")
			return
		}
		current.Role = role
	}
	if in.Active != nil {
		if actor.ID == id && !*in.Active {
			Error(w, 400, "you cannot deactivate your current account")
			return
		}
		if *in.Active {
			current.Active = 1
		} else {
			current.Active = 0
		}
	}
	protected := current.Protected
	if current.Role == "developer" {
		protected = 1
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		Error(w, 500, "could not update staff")
		return
	}
	defer tx.Rollback()
	if strings.TrimSpace(in.Password) != "" {
		if len(in.Password) < 8 || len(in.Password) > 256 {
			Error(w, 400, "password must be at least 8 characters")
			return
		}
		salt, hash, err := createPasswordRecord(in.Password)
		if err != nil {
			Error(w, 500, "could not update password")
			return
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE staff_accounts SET email=?,display_name=?,role=?,active=?,protected=?,password_salt=?,password_hash=?,password_iterations=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			nullIfEmpty(current.Email), current.DisplayName, current.Role, current.Active, protected, salt, hash, passwordIterations, id); err != nil {
			Error(w, 500, "could not update staff")
			return
		}
	} else {
		if _, err = tx.ExecContext(r.Context(), `UPDATE staff_accounts SET email=?,display_name=?,role=?,active=?,protected=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			nullIfEmpty(current.Email), current.DisplayName, current.Role, current.Active, protected, id); err != nil {
			Error(w, 500, "could not update staff")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		Error(w, 500, "could not update staff")
		return
	}
	a.auditAdmin(actor.ID, "staff.updated", fmt.Sprintf("staff:%d", id), r)
	JSON(w, 200, Envelope{"ok": true, "message": "Staff account updated."})
}

func (a *App) handleAdminContacts(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,COALESCE(name,''),COALESCE(email,''),COALESCE(subject,''),COALESCE(message,''),COALESCE(status,'new'),COALESCE(created_at,'') FROM contact_messages ORDER BY CASE status WHEN 'new' THEN 0 WHEN 'read' THEN 1 ELSE 2 END,id DESC LIMIT 200`)
	if err != nil {
		Error(w, 500, "Messages are unavailable right now.")
		return
	}
	defer rows.Close()
	items := []Envelope{}
	for rows.Next() {
		var id int64
		var name, email, subject, message, status, created string
		if err := rows.Scan(&id, &name, &email, &subject, &message, &status, &created); err != nil {
			Error(w, 500, "Messages are unavailable right now.")
			return
		}
		items = append(items, Envelope{"id": id, "name": name, "email": email, "subject": subject, "message": message, "status": status, "created_at": created})
	}
	if err := rows.Err(); err != nil {
		Error(w, 500, "Messages are unavailable right now.")
		return
	}
	JSON(w, 200, Envelope{"ok": true, "messages": items})
}

func (a *App) handleAdminRatings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT r.id,COALESCE(r.display_name,''),COALESCE(v.email,''),r.stars,COALESCE(r.comment,''),r.visible,COALESCE(r.created_at,'') FROM ratings r LEFT JOIN visitor_accounts v ON v.id=r.visitor_id ORDER BY r.id DESC LIMIT 200`)
	if err != nil {
		Error(w, 500, "Reviews are unavailable right now.")
		return
	}
	defer rows.Close()
	items := []Envelope{}
	for rows.Next() {
		var id int64
		var name, email, comment, created string
		var stars, visible int
		if err := rows.Scan(&id, &name, &email, &stars, &comment, &visible, &created); err != nil {
			Error(w, 500, "Reviews are unavailable right now.")
			return
		}
		items = append(items, Envelope{"id": id, "display_name": name, "email": email, "stars": stars, "comment": comment, "visible": visible == 1, "created_at": created})
	}
	if err := rows.Err(); err != nil {
		Error(w, 500, "Reviews are unavailable right now.")
		return
	}
	JSON(w, 200, Envelope{"ok": true, "ratings": items})
}

func (a *App) handleAdminSystem(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var productCount, staffCount, messageCount int
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM products`).Scan(&productCount)
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM staff_accounts`).Scan(&staffCount)
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM contact_messages`).Scan(&messageCount)
	specs := collectSystemSpecs()
	specs["product_rows"] = productCount
	specs["staff_rows"] = staffCount
	specs["message_rows"] = messageCount
	specs["current_role"] = staff.Role
	specs["google_sign_in"] = a.cfg.GoogleOAuthEnabled && a.cfg.GoogleClientID != "" && a.cfg.GoogleClientSecret != ""
	specs["email_service"] = a.cfg.SMTPEnabled && a.cfg.SMTPHost != "" && a.cfg.SMTPUsername != "" && a.cfg.SMTPPassword != ""
	JSON(w, 200, Envelope{"ok": true, "system": specs})
}

func (a *App) auditAdmin(staffID int64, action, target string, r *http.Request) {
	_, _ = a.db.ExecContext(r.Context(), `INSERT INTO audit_logs(staff_id,action,target,ip_address,user_agent) VALUES(?,?,?,?,?)`,
		staffID, action, target, r.RemoteAddr, r.UserAgent())
}

func pathID(path, prefix string) (int64, error) {
	raw := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if strings.Contains(raw, "/") || raw == "" {
		return 0, errors.New("invalid id")
	}
	return strconv.ParseInt(raw, 10, 64)
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.TrimSpace(s)
}
