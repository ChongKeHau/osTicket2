package importer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/auth"
)

// staffEmail returns the address to store and whether it was replaced with a placeholder.
func staffEmail(username, email string, taken map[string]bool) (string, bool) {
	e := strings.TrimSpace(email)
	key := strings.ToLower(e)
	if e == "" || taken[key] {
		p := username + "@imported.invalid"
		taken[strings.ToLower(p)] = true
		return p, true
	}
	taken[key] = true
	return e, false
}

// staffPassword keeps a stored bcrypt hash, or generates an unusable random one.
func staffPassword(passwd string) (string, bool, error) {
	if strings.HasPrefix(passwd, "$2") {
		return passwd, false, nil
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", false, err
	}
	h, err := auth.HashPassword(hex.EncodeToString(b[:]))
	if err != nil {
		return "", false, err
	}
	return h, true, nil
}

func importStaff(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	items, err := src.Staff(ctx)
	if err != nil {
		return err
	}
	access, err := src.StaffDepts(ctx)
	if err != nil {
		return err
	}
	taken := map[string]bool{}
	now := time.Now()
	primary := map[int64]int64{}
	var batch [][]any
	for _, st := range items {
		rep.Read(EntityStaff)
		var tc textCleaner
		st.Username, st.Email = tc.clean(st.Username), tc.clean(st.Email)
		st.FirstName, st.LastName = tc.clean(st.FirstName), tc.clean(st.LastName)
		tc.note(rep, EntityStaff, st.ID)
		email, replaced := staffEmail(st.Username, st.Email, taken)
		if replaced {
			rep.Note(EntityStaff, st.ID, "email replaced with "+email)
		}
		hash, reset, err := staffPassword(st.Passwd)
		if err != nil {
			return err
		}
		if reset {
			rep.Note(EntityStaff, st.ID, "password reset required")
		}
		dept, ok := lk.Departments[st.DeptID]
		if !ok {
			dept = lk.DefaultDept
			rep.Note(EntityStaff, st.ID, "primary department missing, using seed department")
		}
		id := allocIDNoted(lk, rep, EntityStaff, "staff", st.ID)
		lk.Staff[st.ID] = id
		primary[id] = dept
		created := orZero(st.Created, now)
		batch = append(batch, []any{id, st.Username, email, hash, st.FirstName, st.LastName, st.IsAdmin, st.IsActive, dept, created, orZero(st.Updated, created)})
		rep.Written(EntityStaff)
	}
	if err := w.Insert(ctx, "staff", []string{"id", "username", "email", "password_hash", "first_name", "last_name", "is_admin", "is_active", "primary_dept_id", "created_at", "updated_at"}, batch); err != nil {
		return err
	}
	// Memberships: each primary department, plus every access row with a known department.
	type pair struct{ s, d int64 }
	seen := map[pair]bool{}
	var members [][]any
	add := func(s, d int64) {
		if !seen[pair{s, d}] {
			seen[pair{s, d}] = true
			members = append(members, []any{s, d})
		}
	}
	for id, d := range primary {
		add(id, d)
	}
	for _, a := range access {
		s, ok := lk.Staff[a.StaffID]
		if !ok {
			continue
		}
		if d, ok := lk.Departments[a.DeptID]; ok {
			add(s, d)
		}
	}
	return w.Insert(ctx, "staff_department", []string{"staff_id", "dept_id"}, members)
}
