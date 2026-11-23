package internals

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func openDatabase(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	open := func() (*sql.DB, error) {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		if _, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL;`); err != nil {
			_ = db.Close()
			return nil, err
		}
		if err = db.Ping(); err != nil {
			_ = db.Close()
			return nil, err
		}
		var integrity string
		if err = db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
			_ = db.Close()
			return nil, err
		}
		if !strings.EqualFold(strings.TrimSpace(integrity), "ok") {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite integrity check failed: %s", integrity)
		}
		return db, nil
	}

	db, err := open()
	if err == nil {
		return db, nil
	}

	// A damaged development database must not make the whole application
	// impossible to start. Keep the original bytes as a recovery copy and let
	// bootstrap create a fresh database from schema.sql + seed.sql.
	if _, statErr := os.Stat(path); statErr != nil {
		return nil, err
	}
	backup := path + ".corrupt-" + time.Now().Format("20060102-150405")
	if renameErr := os.Rename(path, backup); renameErr != nil {
		return nil, fmt.Errorf("database is unreadable (%v); could not preserve it as %s: %w", err, backup, renameErr)
	}
	// WAL/SHM files belong to the damaged database and cannot be reused.
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	fmt.Printf("[LIXR] damaged database preserved as %s; creating a clean database\n", backup)

	db, retryErr := open()
	if retryErr != nil {
		return nil, fmt.Errorf("create replacement database after preserving %s: %w", backup, retryErr)
	}
	return db, nil
}

func (a *App) bootstrap() error {
	schema, err := os.ReadFile(filepath.Join("..", "database", "schema.sql"))
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if _, err := a.db.Exec(string(schema)); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if err := a.migrateLegacyDatabase(); err != nil {
		return err
	}

	var count int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&count); err != nil {
		return err
	}
	var staffCount int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM staff_accounts WHERE username='toneiu'`).Scan(&staffCount); err != nil {
		return err
	}
	if count == 0 || staffCount == 0 {
		seed, err := os.ReadFile(filepath.Join("..", "database", "seed.sql"))
		if err != nil {
			return fmt.Errorf("read seed: %w", err)
		}
		if _, err := a.db.Exec(string(seed)); err != nil {
			return fmt.Errorf("seed database: %w", err)
		}
	}

	// Remove only the obsolete account created by the previous Lixr development bootstrap.
	_, _ = a.db.Exec(`DELETE FROM staff_accounts
		WHERE lower(COALESCE(email,''))='admin@lixr.local'
		  AND display_name='Lixr Administrator'
		  AND role IN ('administrator','admin')
		  AND username <> 'toneiu'`)
	_, _ = a.db.Exec(`DELETE FROM staff_sessions WHERE staff_id NOT IN (SELECT id FROM staff_accounts)`)
	// Remove the original storefront demo reviews so only visitor-submitted reviews remain.
	_, _ = a.db.Exec(`DELETE FROM ratings WHERE
		(display_name='Mara S.' AND comment='Clean pieces, thoughtful fabric choices, and the store feels as considered as the clothes.') OR
		(display_name='Jules A.' AND comment='The fit guidance was excellent. I left with pieces I can actually wear in different ways.') OR
		(display_name='Nica R.' AND comment='Minimal without feeling plain. The in-store availability on the site is genuinely useful.')`)
	return nil
}

func (a *App) migrateLegacyDatabase() error {
	// SQLite CREATE TABLE IF NOT EXISTS does not add columns. These migrations
	// make older Lixr development databases safe to open with the new source.
	productColumns := map[string]string{
		"sku":                 "TEXT NOT NULL DEFAULT ''",
		"image_source_url":    "TEXT NOT NULL DEFAULT ''",
		"image_data":          "BLOB",
		"image_mime":          "TEXT NOT NULL DEFAULT ''",
		"barcode":             "TEXT NOT NULL DEFAULT ''",
		"color":               "TEXT NOT NULL DEFAULT ''",
		"size":                "TEXT NOT NULL DEFAULT ''",
		"cost_price":          "REAL NOT NULL DEFAULT 0",
		"retail_price":        "REAL NOT NULL DEFAULT 0",
		"sale_price":          "REAL NOT NULL DEFAULT 0",
		"low_stock_threshold": "INTEGER NOT NULL DEFAULT 10",
	}
	for name, definition := range productColumns {
		if err := ensureColumn(a.db, "products", name, definition); err != nil {
			return err
		}
	}
	staffColumns := map[string]string{
		"username":  "TEXT",
		"protected": "INTEGER NOT NULL DEFAULT 0",
	}
	for name, definition := range staffColumns {
		if err := ensureColumn(a.db, "staff_accounts", name, definition); err != nil {
			return err
		}
	}
	ratingColumns := map[string]string{
		"visitor_id": "INTEGER",
	}
	for name, definition := range ratingColumns {
		if err := ensureColumn(a.db, "ratings", name, definition); err != nil {
			return err
		}
	}
	newsletterColumns := map[string]string{
		"unsubscribe_token": "TEXT NOT NULL DEFAULT ''",
		"active":            "INTEGER NOT NULL DEFAULT 1",
		"unsubscribed_at":   "TEXT",
	}
	for name, definition := range newsletterColumns {
		if err := ensureColumn(a.db, "newsletter_subscribers", name, definition); err != nil {
			return err
		}
	}
	_, _ = a.db.Exec(`UPDATE products SET retail_price=price WHERE retail_price=0 AND price>0`)
	_, _ = a.db.Exec(`UPDATE products SET sale_price=price WHERE sale_price=0 AND price>0`)
	_, _ = a.db.Exec(`UPDATE products SET cost_price=ROUND(retail_price*0.42,2) WHERE cost_price=0 AND retail_price>0`)
	_, _ = a.db.Exec(`UPDATE products SET sku='LIXR-' || printf('%04d',id) WHERE trim(COALESCE(sku,''))=''`)
	_, _ = a.db.Exec(`UPDATE products SET image_source_url=image_url WHERE trim(COALESCE(image_source_url,''))='' AND image_url LIKE 'http%'`)
	_, _ = a.db.Exec(`UPDATE newsletter_subscribers SET unsubscribe_token=lower(hex(randomblob(24))) WHERE trim(COALESCE(unsubscribe_token,''))=''`)
	_, _ = a.db.Exec(`INSERT OR IGNORE INTO product_sizes(product_id,size_label,quantity)
		SELECT id, CASE WHEN trim(COALESCE(size,''))='' THEN 'One size' ELSE trim(size) END, stock FROM products`)
	_, _ = a.db.Exec(`UPDATE products SET stock=COALESCE((SELECT SUM(quantity) FROM product_sizes WHERE product_id=products.id),stock)`)
	_, _ = a.db.Exec(`UPDATE staff_accounts SET username=lower(substr(email,1,instr(email,'@')-1)) WHERE (username IS NULL OR trim(username)='') AND instr(email,'@')>1`)
	_, _ = a.db.Exec(`UPDATE staff_accounts SET username='staff-' || id WHERE username IS NULL OR trim(username)=''`)
	_, _ = a.db.Exec(`UPDATE staff_accounts SET role='admin' WHERE role='administrator'`)
	if _, err := a.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_username ON staff_accounts(username COLLATE NOCASE)`); err != nil {
		return fmt.Errorf("create staff username index: %w", err)
	}
	if _, err := a.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_products_sku ON products(sku COLLATE NOCASE)`); err != nil {
		return fmt.Errorf("create product sku index: %w", err)
	}
	return nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if strings.EqualFold(name, column) {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if found {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	if err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}
