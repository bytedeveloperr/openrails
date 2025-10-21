package clickhousemigrations

import (
	"embed"

	log "github.com/sirupsen/logrus"
)

//go:embed *.sql
var FS embed.FS

func init() {
	// Best-effort: log that CH migrations are embedded
	if entries, err := FS.ReadDir("."); err == nil {
		count := 0
		for _, e := range entries {
			if !e.IsDir() {
				count++
			}
		}
		log.WithField("count", count).Info("ClickHouse migrations embedded")
	}
}
