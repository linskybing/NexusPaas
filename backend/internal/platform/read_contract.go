package platform

import "net/http"

// RegisterReadContract exposes a service-key-gated, read-only HTTP contract for one
// resource owned by this service: GET listPath (full list) and GET getPath (single
// record by id). It is the provider side of domainReadContracts — consumers read it
// transparently through crossServiceStore/RemoteServiceReader when the owner is not
// co-hosted, so no bespoke client is needed.
//
// getPath should use a trailing wildcard segment ("{id...}") so composite keys that
// contain "/" (e.g. "<projectID>/<userID>") match, because RemoteServiceReader
// substitutes the raw id into the path without escaping. The captured value is read
// from PathValue("id"). Pass getPath == "" to register a list-only contract.
func (a *App) RegisterReadContract(resource, listPath, getPath string) {
	if a == nil || a.Mux == nil {
		return
	}
	audience := resourceOwner(resource)
	a.Mux.HandleFunc(http.MethodGet+" "+listPath, func(w http.ResponseWriter, r *http.Request) {
		if !a.AuthorizeServiceRequestForAudience(w, r, audience) {
			return
		}
		WriteJSON(w, r, http.StatusOK, a.Store.List(r.Context(), resource))
	})
	if getPath == "" {
		return
	}
	a.Mux.HandleFunc(http.MethodGet+" "+getPath, func(w http.ResponseWriter, r *http.Request) {
		if !a.AuthorizeServiceRequestForAudience(w, r, audience) {
			return
		}
		id := r.PathValue("id")
		record, ok := a.Store.Get(r.Context(), resource, id)
		if !ok {
			WriteJSON(w, r, http.StatusNotFound, map[string]any{"resource": resource, "id": id})
			return
		}
		WriteJSON(w, r, http.StatusOK, record)
	})
}
