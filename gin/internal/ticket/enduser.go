package ticket

import (
	"context"
	"errors"
	"strings"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

// resolveEndUser returns the id of the end user for email, matched
// case-insensitively, creating one (with name) when the address has none. A
// blank name fills in an existing user's blank name, as the portal's own
// upsert does. An empty email resolves to nil: the ticket belongs to no one.
//
// It mirrors client.Service.UpsertByEmail; the client package imports this
// one, so the ticket service cannot call it.
func resolveEndUser(ctx context.Context, q *db.Queries, email, name string) (*int64, error) {
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(name)
	if email == "" {
		return nil, nil
	}
	u, err := q.GetEndUserByEmail(ctx, email)
	switch {
	case err == nil:
		if u.Name == "" && name != "" {
			if u, err = q.UpdateEndUserName(ctx, db.UpdateEndUserNameParams{ID: u.ID, Name: name}); err != nil {
				return nil, err
			}
		}
	case errors.Is(err, pgx.ErrNoRows):
		if u, err = q.CreateEndUser(ctx, db.CreateEndUserParams{Email: email, Name: name}); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	return &u.ID, nil
}
