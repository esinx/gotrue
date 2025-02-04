package models

import (
	"fmt"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	metricglobal "go.opentelemetry.io/otel/metric/global"
	metricinstrument "go.opentelemetry.io/otel/metric/instrument"
	otelasyncint64instrument "go.opentelemetry.io/otel/metric/instrument/asyncint64"

	"github.com/supabase/gotrue/internal/observability"
	"github.com/supabase/gotrue/internal/storage"
)

// cleanupNext holds an atomically incrementing value that determines which of
// the cleanupStatements will be run next.
var cleanupNext uint32

// cleanupStatements holds all of the possible cleanup raw SQL. Only one at a
// time is executed using cleanupNext % len(cleanupStatements).
var CleanupStatements []string

// cleanupAffectedRows tracks an OpenTelemetry metric on the total number of
// cleaned up rows.
var cleanupAffectedRows otelasyncint64instrument.Counter

func init() {
	tableRefreshTokens := RefreshToken{}.TableName()
	tableSessions := Session{}.TableName()
	tableRelayStates := SAMLRelayState{}.TableName()
	tableFlowStates := FlowState{}.TableName()

	// These statements intentionally use SELECT ... FOR UPDATE SKIP LOCKED
	// as this makes sure that only rows that are not being used in another
	// transaction are deleted. These deletes are thus very quick and
	// efficient, as they don't wait on other transactions.
	CleanupStatements = append(CleanupStatements,
		fmt.Sprintf("delete from %q where id in (select id from %q where revoked is true and updated_at < now() - interval '24 hours' limit 100 for update skip locked);", tableRefreshTokens, tableRefreshTokens),
		fmt.Sprintf("update %q set revoked = true, updated_at = now() where id in (select %q.id from %q join %q on %q.session_id = %q.id where %q.not_after < now() - interval '24 hours' and %q.revoked is false limit 100 for update skip locked);", tableRefreshTokens, tableRefreshTokens, tableRefreshTokens, tableSessions, tableRefreshTokens, tableSessions, tableSessions, tableRefreshTokens),
		// sessions are deleted after 72 hours to allow refresh tokens
		// to be deleted piecemeal; 10 at once so that cascades don't
		// overwork the database
		fmt.Sprintf("delete from %q where id in (select id from %q where not_after < now() - interval '72 hours' limit 10 for update skip locked);", tableSessions, tableSessions),
		fmt.Sprintf("delete from %q where id in (select id from %q where created_at < now() - interval '24 hours' limit 100 for update skip locked);", tableRelayStates, tableRelayStates),
		fmt.Sprintf("delete from %q where id in (select id from %q where created_at < now() - interval '24 hours' limit 100 for update skip locked);", tableFlowStates, tableFlowStates),
	)

	var err error
	cleanupAffectedRows, err = metricglobal.Meter("gotrue").AsyncInt64().Counter(
		"gotrue_cleanup_affected_rows",
		metricinstrument.WithDescription("Number of affected rows from cleaning up stale entities"),
	)
	if err != nil {
		logrus.WithError(err).Error("unable to get gotrue.gotrue_cleanup_rows counter metric")
	}
}

// Cleanup removes stale entities in the database. You can call it on each
// request or as a periodic background job. It does quick lockless updates or
// deletes, has an execution timeout and acquire timeout so that cleanups do
// not affect performance of other database jobs. Note that calling this does
// not clean up the whole database, but does a small piecemeal clean up each
// time when called.
func Cleanup(db *storage.Connection) (int, error) {
	ctx, span := observability.Tracer("gotrue").Start(db.Context(), "database-cleanup")
	defer span.End()

	affectedRows := 0
	defer span.SetAttributes(attribute.Int64("gotrue.cleanup.affected_rows", int64(affectedRows)))

	if err := db.WithContext(ctx).Transaction(func(tx *storage.Connection) error {
		nextIndex := atomic.AddUint32(&cleanupNext, 1) % uint32(len(CleanupStatements))
		statement := CleanupStatements[nextIndex]

		count, terr := tx.RawQuery(statement).ExecWithCount()
		if terr != nil {
			return terr
		}

		affectedRows += count

		return nil
	}); err != nil {
		return affectedRows, err
	}

	if cleanupAffectedRows != nil {
		cleanupAffectedRows.Observe(ctx, int64(affectedRows))
	}

	return affectedRows, nil
}
