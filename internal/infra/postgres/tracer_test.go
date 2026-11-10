package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestParseQuery(t *testing.T) {
	const trips = "trips"

	tests := []struct {
		sql            string
		wantOperation  string
		wantCollection string
	}{
		{sql: "INSERT INTO trips (id,user_id) VALUES ($1,$2)", wantOperation: "INSERT", wantCollection: trips},
		{sql: "INSERT INTO trip_status_history (trip_id) VALUES ($1)", wantOperation: "INSERT", wantCollection: "trip_status_history"},
		{sql: "SELECT id, status FROM trips WHERE id = $1 FOR UPDATE", wantOperation: "SELECT", wantCollection: trips},
		{sql: "SELECT count(*) FROM trips WHERE status = $1", wantOperation: "SELECT", wantCollection: trips},
		{sql: "UPDATE trips SET status = $1 WHERE id = $2", wantOperation: "UPDATE", wantCollection: trips},
		{sql: "delete from \"trips\" where id = $1", wantOperation: "DELETE", wantCollection: trips},
		{sql: "begin isolation level read committed", wantOperation: "BEGIN"},
		{sql: "-- ping"},
		{sql: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			op, coll := parseQuery(tt.sql)
			assert.Equal(t, tt.wantOperation, op)
			assert.Equal(t, tt.wantCollection, coll)
		})
	}
}

func TestErrorType(t *testing.T) {
	assert.Equal(t, "23505", errorType(&pgconn.PgError{Code: "23505"}).Value.AsString())
	assert.NotEmpty(t, errorType(errors.New("boom")).Value.AsString())
}
