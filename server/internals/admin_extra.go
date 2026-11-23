package internals

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func parsePathID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("missing id")
	}
	return strconv.ParseInt(raw, 10, 64)
}

type sizeStockInput struct {
	Mode  string `json:"mode"`
	Stock int    `json:"stock"`
	Sizes []struct {
		Label    string `json:"label"`
		Quantity int    `json:"quantity"`
	} `json:"sizes"`
}

func (a *App) handleAdminProductStock(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		Error(w, 400, "Invalid product.")
		return
	}
	if r.Method == http.MethodGet {
		var name string
		var productStock int
		if err := a.db.QueryRowContext(r.Context(), `SELECT name,stock FROM products WHERE id=?`, id).Scan(&name, &productStock); err != nil {
			Error(w, 404, "Product not found.")
			return
		}
		rows, err := a.db.QueryContext(r.Context(), `SELECT size_label,quantity FROM product_sizes WHERE product_id=? ORDER BY id`, id)
		if err != nil {
			Error(w, 500, "Stock details are unavailable right now.")
			return
		}
		sizes := []Envelope{}
		total := 0
		hasSizes := false
		for rows.Next() {
			var label string
			var quantity int
			if err := rows.Scan(&label, &quantity); err != nil {
				_ = rows.Close()
				Error(w, 500, "Stock details are unavailable right now.")
				return
			}
			total += quantity
			if !isDirectStockLabel(label) {
				hasSizes = true
				sizes = append(sizes, Envelope{"label": label, "quantity": quantity})
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			Error(w, 500, "Stock details are unavailable right now.")
			return
		}
		_ = rows.Close()
		if !hasSizes {
			total = productStock
			sizes = []Envelope{}
		}
		JSON(w, 200, Envelope{"ok": true, "product_id": id, "product_name": name, "stock": total, "has_sizes": hasSizes, "sizes": sizes})
		return
	}

	var in sizeStockInput
	if err := decodeJSON(r, &in); err != nil {
		Error(w, 400, "Please check the stock details and try again.")
		return
	}
	in.Mode = strings.ToLower(strings.TrimSpace(in.Mode))
	if in.Mode == "" {
		if len(in.Sizes) > 0 {
			in.Mode = "sizes"
		} else {
			in.Mode = "single"
		}
	}
	if in.Mode != "single" && in.Mode != "sizes" {
		Error(w, 400, "Choose quantity only or stock by size.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		Error(w, 500, "Stock could not be updated.")
		return
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM products WHERE id=?`, id).Scan(&exists); err != nil || exists == 0 {
		Error(w, 404, "Product not found.")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM product_sizes WHERE product_id=?`, id); err != nil {
		Error(w, 500, "Stock could not be updated.")
		return
	}

	total := 0
	sizeSummary := ""
	if in.Mode == "single" {
		if in.Stock < 0 || in.Stock > 1000000 {
			Error(w, 400, "Enter a valid stock quantity.")
			return
		}
		total = in.Stock
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO product_sizes(product_id,size_label,quantity) VALUES(?,'One size',?)`, id, total); err != nil {
			Error(w, 500, "Stock could not be updated.")
			return
		}
	} else {
		if len(in.Sizes) == 0 || len(in.Sizes) > 30 {
			Error(w, 400, "Add at least one size, up to 30 sizes.")
			return
		}
		seen := map[string]bool{}
		for i := range in.Sizes {
			in.Sizes[i].Label = strings.TrimSpace(in.Sizes[i].Label)
			key := strings.ToLower(in.Sizes[i].Label)
			if in.Sizes[i].Label == "" || len(in.Sizes[i].Label) > 40 || isDirectStockLabel(in.Sizes[i].Label) || seen[key] || in.Sizes[i].Quantity < 0 || in.Sizes[i].Quantity > 1000000 {
				Error(w, 400, "Each size needs a unique label and a valid quantity.")
				return
			}
			seen[key] = true
			total += in.Sizes[i].Quantity
		}
		for _, size := range in.Sizes {
			if _, err = tx.ExecContext(r.Context(), `INSERT INTO product_sizes(product_id,size_label,quantity) VALUES(?,?,?)`, id, size.Label, size.Quantity); err != nil {
				Error(w, 500, "Stock could not be updated.")
				return
			}
		}
		sizeSummary = summarizeSizes(in.Sizes)
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE products SET stock=?,size=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, total, sizeSummary, id); err != nil {
		Error(w, 500, "Stock could not be updated.")
		return
	}
	if err = tx.Commit(); err != nil {
		Error(w, 500, "Stock could not be updated.")
		return
	}
	a.auditAdmin(staff.ID, "product.stock", fmt.Sprintf("product:%d", id), r)
	JSON(w, 200, Envelope{"ok": true, "stock": total, "has_sizes": in.Mode == "sizes", "message": "Stock updated."})
}

func summarizeSizes(sizes []struct {
	Label    string `json:"label"`
	Quantity int    `json:"quantity"`
}) string {
	labels := make([]string, 0, len(sizes))
	for _, s := range sizes {
		if s.Quantity > 0 {
			labels = append(labels, s.Label)
		}
	}
	if len(labels) == 0 {
		for _, s := range sizes {
			labels = append(labels, s.Label)
		}
	}
	return strings.Join(labels, ", ")
}

func (a *App) handleAdminDeleteProduct(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		Error(w, 400, "Invalid product.")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM products WHERE id=?`, id)
	if err != nil {
		Error(w, 500, "Product could not be deleted.")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		Error(w, 404, "Product not found.")
		return
	}
	a.auditAdmin(staff.ID, "product.deleted", fmt.Sprintf("product:%d", id), r)
	JSON(w, 200, Envelope{"ok": true, "message": "Product deleted."})
}

func (a *App) handleAdminContactStatus(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		Error(w, 400, "Invalid message.")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if decodeJSON(r, &in) != nil {
		Error(w, 400, "Choose a message status.")
		return
	}
	if in.Status != "new" && in.Status != "read" && in.Status != "resolved" {
		Error(w, 400, "Choose New, Read, or Resolved.")
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE contact_messages SET status=? WHERE id=?`, in.Status, id); err != nil {
		Error(w, 500, "Message could not be updated.")
		return
	}
	a.auditAdmin(staff.ID, "contact.status", fmt.Sprintf("contact:%d", id), r)
	JSON(w, 200, Envelope{"ok": true, "message": "Message status updated."})
}

func (a *App) handleAdminRatingVisibility(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		Error(w, 400, "Invalid review.")
		return
	}
	var in struct {
		Visible bool `json:"visible"`
	}
	if decodeJSON(r, &in) != nil {
		Error(w, 400, "Review could not be updated.")
		return
	}
	visible := 0
	if in.Visible {
		visible = 1
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE ratings SET visible=? WHERE id=?`, visible, id); err != nil {
		Error(w, 500, "Review could not be updated.")
		return
	}
	a.auditAdmin(staff.ID, "rating.visibility", fmt.Sprintf("rating:%d", id), r)
	JSON(w, 200, Envelope{"ok": true, "message": "Review visibility updated."})
}

func (a *App) handleAdminSales(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	page := queryInt(r, "page", 1, 1, 100000)
	limit := queryInt(r, "limit", 20, 5, 100)
	offset := (page - 1) * limit
	var total int
	_ = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sales`).Scan(&total)
	rows, err := a.db.QueryContext(r.Context(), `SELECT s.id,COALESCE(s.receipt_no,''),COALESCE(a.display_name,'—'),s.customer_name,s.customer_email,s.total,s.cost_total,s.payment_method,s.status,s.created_at FROM sales s LEFT JOIN staff_accounts a ON a.id=s.cashier_id ORDER BY s.id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		Error(w, 500, "Sales history is unavailable right now.")
		return
	}
	defer rows.Close()
	items := []Envelope{}
	for rows.Next() {
		var id int64
		var receipt, cashier, customer, email, payment, status, created string
		var totalValue, cost float64
		if rows.Scan(&id, &receipt, &cashier, &customer, &email, &totalValue, &cost, &payment, &status, &created) == nil {
			items = append(items, Envelope{"id": id, "receipt_no": receipt, "cashier": cashier, "customer_name": customer, "customer_email": email, "total": totalValue, "profit": totalValue - cost, "payment_method": payment, "status": status, "created_at": created})
		}
	}
	pages := 0
	if total > 0 {
		pages = (total + limit - 1) / limit
	}
	JSON(w, 200, Envelope{"ok": true, "sales": items, "total": total, "page": page, "pages": pages})
}

func (a *App) handleAdminReceipt(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		Error(w, 400, "Invalid sale.")
		return
	}
	var receipt, cashier, customer, email, payment, status, created string
	var subtotal, discount, totalValue, cost float64
	err = a.db.QueryRowContext(r.Context(), `SELECT COALESCE(s.receipt_no,''),COALESCE(a.display_name,'—'),s.customer_name,s.customer_email,s.subtotal,s.discount,s.total,s.cost_total,s.payment_method,s.status,s.created_at FROM sales s LEFT JOIN staff_accounts a ON a.id=s.cashier_id WHERE s.id=?`, id).
		Scan(&receipt, &cashier, &customer, &email, &subtotal, &discount, &totalValue, &cost, &payment, &status, &created)
	if errors.Is(err, sql.ErrNoRows) {
		Error(w, 404, "Sale not found.")
		return
	}
	if err != nil {
		Error(w, 500, "Receipt is unavailable right now.")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT product_name,sku,size_label,quantity,unit_price,line_total FROM sale_items WHERE sale_id=? ORDER BY id`, id)
	if err != nil {
		Error(w, 500, "Receipt is unavailable right now.")
		return
	}
	defer rows.Close()
	items := []Envelope{}
	for rows.Next() {
		var name, sku, size string
		var qty int
		var unit, line float64
		if rows.Scan(&name, &sku, &size, &qty, &unit, &line) == nil {
			items = append(items, Envelope{"name": name, "sku": sku, "size": size, "quantity": qty, "unit_price": unit, "line_total": line})
		}
	}
	JSON(w, 200, Envelope{"ok": true, "receipt": Envelope{"id": id, "receipt_no": receipt, "cashier": cashier, "customer_name": customer, "customer_email": email, "subtotal": subtotal, "discount": discount, "total": totalValue, "profit": totalValue - cost, "payment_method": payment, "status": status, "created_at": created, "items": items}})
}

func (a *App) handleAdminDatabase(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	tables, err := a.databaseTables(r)
	if err != nil {
		Error(w, 500, "Database view is unavailable right now.")
		return
	}
	table := strings.TrimSpace(r.URL.Query().Get("table"))
	if table == "" && len(tables) > 0 {
		table = tables[0].Name
	}
	if table != "" && !tableAllowed(tables, table) {
		Error(w, 400, "Unknown table.")
		return
	}
	page := queryInt(r, "page", 1, 1, 100000)
	limit := queryInt(r, "limit", 50, 10, 100)
	data := Envelope{"ok": true, "role": staff.Role, "can_modify": staff.Role == "developer", "tables": tables, "table": table, "page": page, "rows": []any{}, "columns": []string{}}
	if table == "" {
		JSON(w, 200, data)
		return
	}
	columns, rows, total, err := a.readDatabaseTable(r, table, page, limit)
	if err != nil {
		Error(w, 500, "Table could not be opened.")
		return
	}
	pages := 0
	if total > 0 {
		pages = (total + limit - 1) / limit
	}
	data["columns"] = columns
	data["rows"] = rows
	data["total"] = total
	data["pages"] = pages
	JSON(w, 200, data)
}

type dbTableInfo struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

func (a *App) databaseTables(r *http.Request) ([]dbTableInfo, error) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := make([]dbTableInfo, 0, len(names))
	for _, name := range names {
		var count int
		if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM `+quoteIdentifier(name)).Scan(&count); err != nil {
			return nil, err
		}
		out = append(out, dbTableInfo{Name: name, Rows: count})
	}
	return out, nil
}

func tableAllowed(tables []dbTableInfo, table string) bool {
	for _, t := range tables {
		if t.Name == table {
			return true
		}
	}
	return false
}

func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }

func (a *App) readDatabaseTable(r *http.Request, table string, page, limit int) ([]string, []map[string]any, int, error) {
	var total int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM `+quoteIdentifier(table)).Scan(&total); err != nil {
		return nil, nil, 0, err
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT * FROM `+quoteIdentifier(table)+` LIMIT ? OFFSET ?`, limit, (page-1)*limit)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, 0, err
	}
	out := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		item := map[string]any{}
		for i, col := range cols {
			switch value := values[i].(type) {
			case []byte:
				if len(value) > 256 {
					item[col] = fmt.Sprintf("[stored image · %.1f KB]", float64(len(value))/1024)
				} else {
					item[col] = string(value)
				}
			default:
				item[col] = value
			}
		}
		out = append(out, item)
	}
	return cols, out, total, nil
}

func (a *App) handleAdminDatabaseQuery(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if staff.Role != "developer" {
		Error(w, 403, "Database changes are available only to the developer account.")
		return
	}
	var in struct {
		SQL string `json:"sql"`
	}
	if decodeJSON(r, &in) != nil {
		Error(w, 400, "Enter a database statement.")
		return
	}
	statement := strings.TrimSpace(in.SQL)
	if statement == "" || len(statement) > 12000 || strings.Count(strings.TrimSuffix(statement, ";"), ";") > 0 {
		Error(w, 400, "Run one statement at a time.")
		return
	}
	first := strings.ToUpper(strings.Fields(statement)[0])
	allowed := map[string]bool{"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true}
	if !allowed[first] {
		Error(w, 400, "Use SELECT, INSERT, UPDATE, or DELETE here.")
		return
	}
	if first == "SELECT" {
		rows, err := a.db.QueryContext(r.Context(), statement)
		if err != nil {
			Error(w, 400, err.Error())
			return
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		result := []map[string]any{}
		for rows.Next() && len(result) < 200 {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if rows.Scan(ptrs...) != nil {
				break
			}
			item := map[string]any{}
			for i, col := range cols {
				if b, ok := values[i].([]byte); ok {
					item[col] = fmt.Sprintf("[%d bytes]", len(b))
				} else {
					item[col] = values[i]
				}
			}
			result = append(result, item)
		}
		a.auditAdmin(staff.ID, "database.select", "query", r)
		JSON(w, 200, Envelope{"ok": true, "columns": cols, "rows": result, "message": fmt.Sprintf("%d row(s) returned.", len(result))})
		return
	}
	res, err := a.db.ExecContext(r.Context(), statement)
	if err != nil {
		Error(w, 400, err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	a.auditAdmin(staff.ID, "database.modify", strings.ToLower(first), r)
	JSON(w, 200, Envelope{"ok": true, "affected": affected, "message": fmt.Sprintf("%d row(s) changed.", affected)})
}

func (a *App) handleAdminDatabaseReset(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if staff.Role != "developer" {
		Error(w, 403, "Database reset is available only to the developer account.")
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if decodeJSON(r, &in) != nil || in.Confirm != "RESET LIXR" {
		Error(w, 400, "Type RESET LIXR to confirm.")
		return
	}
	seed, err := os.ReadFile(filepath.Join("..", "database", "seed.sql"))
	if err != nil {
		Error(w, 500, "Store data could not be reset.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		Error(w, 500, "Store data could not be reset.")
		return
	}
	defer tx.Rollback()
	for _, table := range []string{"sale_items", "sales", "page_visits", "chat_messages", "chat_threads", "visitor_sessions", "visitor_accounts", "ratings", "contact_messages", "newsletter_subscribers", "product_sizes", "products"} {
		if _, err = tx.ExecContext(r.Context(), `DELETE FROM `+quoteIdentifier(table)); err != nil {
			Error(w, 500, "Store data could not be reset.")
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), string(seed)); err != nil {
		Error(w, 500, "Store data could not be reset.")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO product_sizes(product_id,size_label,quantity) SELECT id,CASE WHEN trim(COALESCE(size,''))='' THEN 'One size' ELSE trim(size) END,stock FROM products`); err != nil {
		Error(w, 500, "Store data could not be reset.")
		return
	}
	if err = tx.Commit(); err != nil {
		Error(w, 500, "Store data could not be reset.")
		return
	}
	a.auditAdmin(staff.ID, "database.reset", "store-data", r)
	JSON(w, 200, Envelope{"ok": true, "message": "Store data reset completed."})
}
