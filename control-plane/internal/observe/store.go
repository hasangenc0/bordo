package observe

import (
	"database/sql"
	"os"
)

// LoadBackends reads all regions from the DB and returns their observe backend URLs.
// Falls back to BORDO_VM_URL / BORDO_LOKI_URL / BORDO_TEMPO_URL env vars when the DB
// has no rows or the columns are not yet populated.
func LoadBackends(db *sql.DB) ([]RegionBackend, error) {
	rows, err := db.Query(
		`SELECT name, COALESCE(vm_url,''), COALESCE(loki_url,''), COALESCE(tempo_url,'')
		 FROM regions WHERE status != 'unreachable'`)
	if err != nil {
		// Migration may not have run yet — fall through to env fallback.
		return envFallback(), nil
	}
	defer rows.Close()

	var backends []RegionBackend
	for rows.Next() {
		var b RegionBackend
		if err := rows.Scan(&b.Region, &b.VMURL, &b.LokiURL, &b.TempoURL); err != nil {
			continue
		}
		backends = append(backends, b)
	}
	if len(backends) == 0 {
		return envFallback(), nil
	}
	return backends, nil
}

// envFallback constructs a single synthetic backend from env vars.
func envFallback() []RegionBackend {
	vmURL := os.Getenv("BORDO_VM_URL")
	lokiURL := os.Getenv("BORDO_LOKI_URL")
	tempoURL := os.Getenv("BORDO_TEMPO_URL")
	if vmURL == "" && lokiURL == "" && tempoURL == "" {
		return nil
	}
	return []RegionBackend{{
		Region:   "default",
		VMURL:    vmURL,
		LokiURL:  lokiURL,
		TempoURL: tempoURL,
	}}
}
