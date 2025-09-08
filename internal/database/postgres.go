package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"
)

// DEĞİŞİKLİK: Fonksiyon artık context alıyor ve context-aware bekleme yapıyor.
func Connect(ctx context.Context, url string, log zerolog.Logger) (*sql.DB, error) {
	var db *sql.DB
	var err error

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("PostgreSQL URL parse edilemedi: %w", err)
	}

	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	finalURL := stdlib.RegisterConnConfig(config.ConnConfig)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		db, err = sql.Open("pgx", finalURL)
		if err == nil {
			db.SetConnMaxLifetime(time.Minute * 3)
			db.SetMaxIdleConns(5)
			db.SetMaxOpenConns(10)

			if pingErr := db.Ping(); pingErr == nil {
				log.Info().Msg("Veritabanına bağlantı başarılı (Simple Protocol Mode).")
				return db, nil
			} else {
				err = pingErr
			}
		}

		if ctx.Err() == nil {
			log.Warn().Err(err).Msg("Veritabanına bağlanılamadı, 5 saniye sonra tekrar denenecek...")
		}

		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// DEĞİŞİKLİK: Bu servisle ilgisi olmayan fonksiyonlar kaldırıldı.
