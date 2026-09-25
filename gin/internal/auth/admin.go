package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

// CreateAdmin creates an active admin in the first department. Used by the
// CLI. firstName defaults to username when empty; lastName defaults to "".
func CreateAdmin(ctx context.Context, b db.Beginner, username, email, password, firstName, lastName string) (int64, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	if firstName == "" {
		firstName = username
	}
	q := db.New(b)
	dept, err := q.FirstDepartment(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("no department exists; run migrations first")
	}
	if err != nil {
		return 0, err
	}
	st, err := q.CreateStaff(ctx, db.CreateStaffParams{
		Username: username, Email: email, PasswordHash: hash, FirstName: firstName, LastName: lastName,
		IsAdmin: true, IsActive: true, PrimaryDeptID: dept.ID,
	})
	if db.IsUniqueViolation(err) {
		return 0, fmt.Errorf("%w: username or email already exists", apperr.ErrConflict)
	}
	if err != nil {
		return 0, err
	}
	return st.ID, nil
}
