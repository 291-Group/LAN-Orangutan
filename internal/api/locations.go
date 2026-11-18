package api

import (
	"encoding/json"
	"net/http"
)

// handleLocations handles GET/POST /api/locations, the list of places a device
// can be. It works like /api/groups: POST takes an action of add, rename or
// delete. A rename moves every device in that location; a delete clears it from
// them and keeps the devices.
func (h *Handler) handleLocations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.success(w, h.locationsView())

	case http.MethodPost:
		var body struct {
			Action  string `json:"action"`
			Name    string `json:"name"`
			NewName string `json:"new_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}

		var err error
		switch body.Action {
		case "add":
			err = h.store.AddLocation(body.Name)
		case "rename":
			err = h.store.RenameLocation(body.Name, body.NewName)
		case "delete":
			err = h.store.DeleteLocation(body.Name)
		default:
			h.fail(w, http.StatusBadRequest, invalidAction("add, rename, delete"))
			return
		}
		if err != nil {
			h.fail(w, http.StatusBadRequest, err)
			return
		}
		h.success(w, h.locationsView())

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) locationsView() map[string]interface{} {
	return map[string]interface{}{
		"locations": nonNil(h.store.GetLocations()),
		"counts":    h.store.LocationCounts(),
	}
}
