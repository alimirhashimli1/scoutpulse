// Command import-transfermarkt loads the open Transfermarkt dataset into the
// football database: the top leagues' clubs, current squads, transfer
// histories, market values and season statistics.
//
//	go run ./cmd/import-transfermarkt -download -dry-run
//	go run ./cmd/import-transfermarkt
//
// The database is DATABASE_URL when set (a full postgres:// URL, which is what
// Supabase and Railway hand out), otherwise the DB_HOST/DB_USER/... variables
// the service itself reads. Neither is ever taken as a flag, so a password
// does not end up in shell history.
//
// Safe to re-run: see package importer.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/scoutpulse/football-svc/internal/importer"
	"github.com/scoutpulse/libs/db"
)

func main() {
	dataDir := flag.String("data", "transfermarkt-data", "directory holding the dataset's .csv.gz files")
	download := flag.Bool("download", false, "fetch the latest dataset into -data first")
	leagues := flag.String("leagues", strings.Join(importer.DefaultLeagues, ","), "Transfermarkt competition ids to import")
	firstSeason := flag.Int("first-season", 2022, "earliest season (starting year) to import stats for")
	dryRun := flag.Bool("dry-run", false, "do everything, report, then roll back")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	scope := importer.Scope{
		Leagues:     strings.Split(*leagues, ","),
		FirstSeason: *firstSeason,
		Today:       time.Now().UTC().Format("2006-01-02"),
	}
	for _, l := range scope.Leagues {
		if c, ok := importer.Catalog[l]; !ok || c.League != "" {
			log.Fatalf("unknown league %q; known: see importer.Catalog", l)
		}
	}

	if *download {
		if err := fetch(ctx, *dataDir, logger); err != nil {
			log.Fatalf("download: %v", err)
		}
	}

	started := time.Now()
	data, err := importer.Load(*dataDir, scope)
	if err != nil {
		log.Fatalf("read dataset: %v", err)
	}
	logger.Info("dataset read", "clubs", len(data.Clubs), "players", len(data.Players),
		"season", data.LatestSeason, "took", time.Since(started).Round(time.Millisecond))

	database, err := connect()
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer func() { _ = database.Close() }()

	report, err := importer.Import(ctx, database, data, scope, *dryRun, logger)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	print(report, *dryRun)
}

func connect() (*sqlx.DB, error) {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		database, err := sqlx.Connect("postgres", url)
		if err != nil {
			return nil, err
		}
		database.SetMaxOpenConns(2)
		return database, nil
	}
	return db.Connect(db.LoadConfigFromEnv())
}

func fetch(ctx context.Context, dir string, logger *slog.Logger) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	for _, name := range importer.DatasetFiles {
		file := name + ".csv.gz"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, importer.DatasetURL+file, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return fmt.Errorf("%s: %s", file, resp.Status)
		}
		// Written beside the target and renamed, so an interrupted download
		// never leaves a truncated file that a later run would read as whole.
		tmp := filepath.Join(dir, file+".part")
		//nolint:gosec // G304: the directory is the operator's own -data flag
		out, err := os.Create(tmp)
		if err != nil {
			_ = resp.Body.Close()
			return err
		}
		n, err := io.Copy(out, resp.Body)
		_ = resp.Body.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(dir, file)); err != nil {
			return err
		}
		logger.Info("downloaded", "file", file, "bytes", n)
	}
	return nil
}

func print(r *importer.Report, dryRun bool) {
	if dryRun {
		fmt.Println("DRY RUN -- nothing was saved.")
	}
	fmt.Printf(`
competitions  %d created, %d matched existing
seasons       %d created
clubs         %d created, %d matched existing
players       %d created, %d matched existing
transfers     %d created, %d updated, %d matched to ones entered by hand
              %d placeholder arrivals replaced by real history
values        %d market value points
stats         %d season lines
entries       %d club-competition-season entries
`,
		r.LeaguesCreated, r.LeaguesMatched, r.SeasonsCreated,
		r.ClubsCreated, r.ClubsMatched, r.PlayersCreated, r.PlayersMatched,
		r.TransfersCreated, r.TransfersUpdated, r.TransfersAdopted, r.OpeningsRemoved,
		r.Valuations, r.StatLines, r.Entries)

	if len(r.ClubsNotInSource) > 0 {
		fmt.Printf("\nClubs filed under an imported league that the source does not have there now (left alone):\n  %s\n",
			strings.Join(r.ClubsNotInSource, "\n  "))
	}
	if len(r.PlayersNotInSource) > 0 {
		fmt.Printf("\nPlayers at an imported club whom the source does not list in that squad (left alone):\n  %s\n",
			strings.Join(r.PlayersNotInSource, "\n  "))
	}
}
