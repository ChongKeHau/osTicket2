package auth

import "github.com/gin-gonic/gin"

// Principal is the authenticated agent for one request.
type Principal struct {
	StaffID int64
	IsAdmin bool
	DeptIDs []int64
}

func (p Principal) CanSeeDept(id int64) bool {
	if p.IsAdmin {
		return true
	}
	for _, d := range p.DeptIDs {
		if d == id {
			return true
		}
	}
	return false
}

const principalKey = "auth.principal"

func WithPrincipal(c *gin.Context, p Principal) { c.Set(principalKey, p) }

func FromContext(c *gin.Context) (Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}
