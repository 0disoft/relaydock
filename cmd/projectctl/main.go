package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/persistence/postgres"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type projectRecord struct {
	OrganizationID   string `json:"organizationId"`
	OrganizationSlug string `json:"organizationSlug"`
	OrganizationName string `json:"organizationName"`
	ProjectID        string `json:"projectId"`
	ProjectSlug      string `json:"projectSlug"`
	ProjectName      string `json:"projectName"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ensure":
		err = ensure(os.Args[2:])
	case "get":
		err = get(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func ensure(args []string) error {
	flags := flag.NewFlagSet("ensure", flag.ContinueOnError)
	organizationSlug := flags.String("organization-slug", "default", "stable lowercase organization slug")
	organizationName := flags.String("organization-name", "Default Organization", "organization display name")
	projectSlug := flags.String("project-slug", "default", "stable lowercase project slug")
	projectName := flags.String("project-name", "Default Project", "project display name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := validateSlug("organization", *organizationSlug); err != nil {
		return err
	}
	if err := validateSlug("project", *projectSlug); err != nil {
		return err
	}
	if strings.TrimSpace(*organizationName) == "" || strings.TrimSpace(*projectName) == "" {
		return fmt.Errorf("organization and project names cannot be empty")
	}

	database, closeDatabase, err := openDatabase()
	if err != nil {
		return err
	}
	defer closeDatabase()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin project bootstrap transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	record := projectRecord{
		OrganizationSlug: strings.TrimSpace(*organizationSlug),
		OrganizationName: strings.TrimSpace(*organizationName),
		ProjectSlug:      strings.TrimSpace(*projectSlug),
		ProjectName:      strings.TrimSpace(*projectName),
	}
	if err := tx.QueryRowContext(ctx, `
INSERT INTO control.organizations (slug, name)
VALUES ($1, $2)
ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
RETURNING id::text, slug, name`, record.OrganizationSlug, record.OrganizationName).Scan(
		&record.OrganizationID,
		&record.OrganizationSlug,
		&record.OrganizationName,
	); err != nil {
		return fmt.Errorf("ensure organization: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
INSERT INTO control.projects (organization_id, slug, name)
VALUES ($1::uuid, $2, $3)
ON CONFLICT (organization_id, slug) DO UPDATE SET name = EXCLUDED.name
RETURNING id::text, slug, name`, record.OrganizationID, record.ProjectSlug, record.ProjectName).Scan(
		&record.ProjectID,
		&record.ProjectSlug,
		&record.ProjectName,
	); err != nil {
		return fmt.Errorf("ensure project: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project bootstrap: %w", err)
	}
	return writeJSON(record)
}

func get(args []string) error {
	flags := flag.NewFlagSet("get", flag.ContinueOnError)
	organizationSlug := flags.String("organization-slug", "default", "organization slug")
	projectSlug := flags.String("project-slug", "default", "project slug")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := validateSlug("organization", *organizationSlug); err != nil {
		return err
	}
	if err := validateSlug("project", *projectSlug); err != nil {
		return err
	}
	database, closeDatabase, err := openDatabase()
	if err != nil {
		return err
	}
	defer closeDatabase()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var record projectRecord
	if err := database.QueryRowContext(ctx, `
SELECT organization.id::text, organization.slug, organization.name,
       project.id::text, project.slug, project.name
FROM control.organizations AS organization
JOIN control.projects AS project ON project.organization_id = organization.id
WHERE organization.slug = $1 AND project.slug = $2`, strings.TrimSpace(*organizationSlug), strings.TrimSpace(*projectSlug)).Scan(
		&record.OrganizationID,
		&record.OrganizationSlug,
		&record.OrganizationName,
		&record.ProjectID,
		&record.ProjectSlug,
		&record.ProjectName,
	); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("project not found")
		}
		return fmt.Errorf("get project: %w", err)
	}
	return writeJSON(record)
}

func openDatabase() (*sql.DB, func(), error) {
	databaseURL := firstNonEmpty(os.Getenv("ARG_POSTGRES_URL"), os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("ARG_POSTGRES_URL or DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	database, err := postgres.OpenSQL(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}
	return database, func() { _ = database.Close() }, nil
}

func validateSlug(kind, value string) error {
	value = strings.TrimSpace(value)
	if !slugPattern.MatchString(value) {
		return fmt.Errorf("%s slug must contain lowercase letters, digits, or internal hyphens and be at most 63 characters", kind)
	}
	return nil
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: projectctl ensure|get [flags]")
}
