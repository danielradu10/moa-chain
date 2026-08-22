package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"moa-chain/explorer"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.GetHealth())
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	block, ok := s.svc.GetBlock(hash)
	if !ok {
		writeError(w, http.StatusNotFound, "block not found")
		return
	}

	writeJSON(w, http.StatusOK, block)
}

func (s *Server) handleRound(w http.ResponseWriter, r *http.Request) {
	round, err := strconv.ParseUint(r.PathValue("round"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid round number")
		return
	}

	resp, ok := s.svc.GetRound(round)
	if !ok {
		writeError(w, http.StatusNotFound, "round not found")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, explorer.ErrorResponse{Error: msg})
}
