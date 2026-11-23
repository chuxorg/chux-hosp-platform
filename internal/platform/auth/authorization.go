package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

const permissionModel = `
[request_definition]
r = permission
[policy_definition]
p = permission
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.permission == p.permission
`

func (h *Handler) ProtectProperty(store *Store, permission string, next func(http.ResponseWriter, *http.Request, int64)) http.Handler {
	return securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, csrfCookieToken, ok := h.authenticate(w, r, false)
		if !ok {
			return
		}
		if r.Method == http.MethodPost {
			csrf := r.Header.Get("X-CSRF-Token")
			if csrf == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(csrfCookieToken)) != 1 || !validOrigin(r) {
				h.problem(w, http.StatusForbidden, "CSRF validation failed")
				return
			}
		}
		propertyID, err := strconv.ParseInt(r.PathValue("propertyID"), 10, 64)
		if err != nil || propertyID <= 0 {
			h.problem(w, http.StatusBadRequest, "invalid property")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), h.config.DatabaseTimeout)
		allowed, err := store.AllowsProperty(ctx, session.ID, propertyID, permission)
		cancel()
		if err != nil {
			h.problem(w, http.StatusServiceUnavailable, "authorization unavailable")
			return
		}
		if !allowed {
			h.problem(w, http.StatusForbidden, "permission denied")
			return
		}
		next(w, r, session.ID)
	}))
}

func (s *Store) AllowsProperty(ctx context.Context, userID, propertyID int64, permission string) (bool, error) {
	permissions, err := s.propertyPermissions(ctx, userID, propertyID)
	if err != nil {
		return false, err
	}
	return evaluatePermission(permission, permissions)
}

func evaluatePermission(permission string, permissions []string) (bool, error) {
	m, err := model.NewModelFromString(permissionModel)
	if err != nil {
		return false, err
	}
	evaluator, err := casbin.NewEnforcer(m)
	if err != nil {
		return false, err
	}
	for _, granted := range permissions {
		if _, err := evaluator.AddPolicy(granted); err != nil {
			return false, err
		}
	}
	return evaluator.Enforce(permission)
}
