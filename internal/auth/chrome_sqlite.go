package auth

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func readChromeCookiesFromDBImpl(path string) (Store, error) {
	// Chrome holds a write lock; open read-only immutable if possible.
	dsn := fmt.Sprintf("file:%s?mode=ro", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return Store{}, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT host_key, name, value FROM cookies`)
	if err != nil {
		return Store{}, err
	}
	defer rows.Close()

	st := Store{Cookies: map[string]string{}, Source: "chrome"}
	for rows.Next() {
		var host, name, value string
		if err := rows.Scan(&host, &name, &value); err != nil {
			return Store{}, err
		}
		if name == "" || value == "" {
			continue
		}
		if !relevantChromeHost(host) {
			continue
		}
		st.Cookies[name] = value
	}
	return st, rows.Err()
}
