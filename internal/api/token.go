package api

import (
	"net/http"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
)

// handleAPIToken handles GET/POST/DELETE /api/token.
//
//	GET    reports whether a token exists. It never returns the token itself.
//	POST   creates one, replacing any previous token, and returns it ONCE.
//	DELETE revokes it.
//
// The token is shown exactly once, at the moment it is created, because only
// its hash is stored. That is the point: a copy of the data directory, a backup
// or a stolen disk does not hand anyone API access. The cost is that a token
// which is lost has to be replaced rather than looked up, which is the right
// trade for a credential that can read every device on someone's network.
func (h *Handler) handleAPIToken(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		h.error(w, http.StatusServiceUnavailable, "authentication is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.success(w, map[string]any{"exists": h.auth.HasAPIToken()})

	case http.MethodPost:
		token, hash, err := auth.NewAPIToken()
		if err != nil {
			h.error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := auth.SaveAPITokenHash(h.cfg.APITokenFile(), hash); err != nil {
			h.error(w, http.StatusInternalServerError, err.Error())
			return
		}
		h.auth.SetAPITokenHash(hash)
		h.success(w, map[string]any{
			"token": token,
			"note":  "Copy this now. It is stored only as a hash, so it cannot be shown again.",
		})

	case http.MethodDelete:
		if err := auth.SaveAPITokenHash(h.cfg.APITokenFile(), ""); err != nil {
			h.error(w, http.StatusInternalServerError, err.Error())
			return
		}
		h.auth.SetAPITokenHash("")
		h.success(w, map[string]string{"message": "API token revoked"})

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
