package db

import "database/sql"

// GetConfig returns the value for a config key, or "" when absent.
func GetConfig(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow("SELECT value FROM app_config WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetConfig sets a config key-value pair.
func SetConfig(db *sql.DB, key, value string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO app_config (key, value) VALUES (?, ?)", key, value)
	return err
}

// GetAllConfigs returns all config key-value pairs.
func GetAllConfigs(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM app_config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configs := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		configs[k] = v
	}
	return configs, rows.Err()
}

// DeleteConfig removes a config key entirely. Used by tooling (cmd/abeval)
// that temporarily flips a key and must restore "absent", not just a value.
func DeleteConfig(db *sql.DB, key string) error {
	_, err := db.Exec("DELETE FROM app_config WHERE key = ?", key)
	return err
}
