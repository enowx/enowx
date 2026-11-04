package library

import (
	"net/http"

	"github.com/enowx/enowx/cloud/api"
)

// picture is a live version's picture for anyone, as the website's pages show it
// to anyone already. Only pictures: a document holds other people's scripts and
// reaches a browser from the website's own address alone.
func (s *Service) picture(w http.ResponseWriter, r *http.Request) {
	if part := r.PathValue("part"); part != api.PartThumb && part != api.PartThumbPhone {
		fail(w, http.StatusNotFound, "not found")
		return
	}
	s.object(w, r)
}
