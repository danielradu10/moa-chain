package controllers

import (
	"encoding/json"
	"fmt"
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

func (s *Server) handleRoundCurrent(w http.ResponseWriter, r *http.Request) {
	resp, ok := s.svc.GetLiveRound()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "round loop not started")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRoundStream(w http.ResponseWriter, r *http.Request) {
	ch, unsub, ok := s.svc.StreamLiveRound()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "round hub not started")
		return
	}
	defer unsub()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-ch:
			if !open {
				return
			}
			payload, err := json.Marshal(s.svc.MakeLiveRoundResponse(event))
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handleTransactions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.GetTransactions())
}

func (s *Server) handleTransaction(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	resp, ok := s.svc.GetTransaction(hash)
	if !ok {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	writeJSON(w, http.StatusOK, resp)
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

func (s *Server) handleTxStream(w http.ResponseWriter, r *http.Request) {
	hexHash := r.PathValue("hash")
	ch, unsub, ok := s.svc.StreamTxEvents(hexHash)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "tx hub not started")
		return
	}
	defer unsub()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-ch:
			if !open {
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, explorer.ErrorResponse{Error: msg})
}
