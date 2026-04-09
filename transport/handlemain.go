package transport

import (
	"aflujo/model"
	"aflujo/sevice"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type HandleMain struct {
	Service *service.Service
}

func New(s *service.Service) *HandleMain {
	return &HandleMain{Service: s}
}

func (h *HandleMain) HandleGetAllMains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.Header.Get("token") != "to" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()

		// Optional filters (combinables).
		var (
			fromDate *time.Time
			max      *int
			ord      string
			avail    *bool
			cats     []string
		)

		if v := strings.TrimSpace(q.Get("fromdate")); v != "" {
			t, err := time.ParseInLocation("2006-01-02", v, time.UTC)
			if err != nil {
				http.Error(w, "invalid fromdate (expected YYYY-MM-DD)", http.StatusBadRequest)
				return
			}
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			fromDate = &t
		}

		if v := strings.TrimSpace(q.Get("category")); v != "" {
			parts := strings.Split(v, ",")
			cats = make([]string, 0, len(parts))
			for _, p := range parts {
				c := strings.TrimSpace(p)
				if c != "" {
					cats = append(cats, c)
				}
			}
		}

		if v := strings.TrimSpace(q.Get("avariable")); v != "" {
			var b bool
			switch v {
			case "1":
				b = true
			case "0":
				b = false
			default:
				parsed, err := strconv.ParseBool(v)
				if err != nil {
					http.Error(w, "invalid avariable (expected true/false/1/0)", http.StatusBadRequest)
					return
				}
				b = parsed
			}
			avail = &b
		}

		if v := strings.TrimSpace(q.Get("max")); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				http.Error(w, "invalid max (expected positive integer)", http.StatusBadRequest)
				return
			}
			if n > 1000 {
				n = 1000
			}
			max = &n
		}

		ord = strings.ToLower(strings.TrimSpace(q.Get("ord")))
		if ord == "" {
			ord = "desc"
		}
		if ord != "asc" && ord != "desc" {
			http.Error(w, "invalid ord (expected asc|desc)", http.StatusBadRequest)
			return
		}

		list, err := h.Service.GetAllFiltered(fromDate, cats, avail, max, ord)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := model.Response{
			Status:    "success",
			Total:     len(list),
			Advice:    "Se recuperaron correctamente los registros disponibles.",
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     list,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// mainCreateBody arma un Maindb desde JSON: obligatorios category y subtype (no vacíos);
// id y created_at opcionales; si available no viene, se asume true; si data no viene, "".
type mainCreateBody struct {
	ID        string     `json:"id"`
	CreatedAt *time.Time `json:"created_at"`
	Category  string     `json:"category"`
	Subtype   string     `json:"subtype"`
	Data      *string    `json:"data"`
	Available *bool      `json:"available"`
}

// adviceDuplicateMainCreate describe si el alta enviada coincide con el registro existente o lista diferencias por campo.
func adviceDuplicateMainCreate(body *mainCreateBody, candidate *model.Maindb, existing *model.Maindb) string {
	rfc := func(t time.Time) string {
		return t.UTC().Truncate(time.Second).Format(time.RFC3339)
	}
	var diffs []string
	if candidate.Category != existing.Category {
		diffs = append(diffs, fmt.Sprintf("category: enviado %q, existente %q", candidate.Category, existing.Category))
	}
	if candidate.Subtype != existing.Subtype {
		diffs = append(diffs, fmt.Sprintf("subtype: enviado %q, existente %q", candidate.Subtype, existing.Subtype))
	}
	if candidate.Data != existing.Data {
		diffs = append(diffs, fmt.Sprintf("data: enviado %q, existente %q", candidate.Data, existing.Data))
	}
	if candidate.Available != existing.Available {
		diffs = append(diffs, fmt.Sprintf("available: enviado %v, existente %v", candidate.Available, existing.Available))
	}
	if body.CreatedAt != nil {
		tsCand := candidate.CreatedAt.UTC().Truncate(time.Second)
		tsEx := existing.CreatedAt.UTC().Truncate(time.Second)
		if !tsCand.Equal(tsEx) {
			diffs = append(diffs, fmt.Sprintf("created_at: enviado %s, existente %s", rfc(candidate.CreatedAt), rfc(existing.CreatedAt)))
		}
	}
	if len(diffs) == 0 {
		if body.CreatedAt == nil {
			return fmt.Sprintf(
				"Los datos enviados coinciden con el registro existente (category, subtype, data, available). "+
					"No se envió created_at; en un alta nueva se usaría la hora actual. El existente tiene created_at %s.",
				rfc(existing.CreatedAt),
			)
		}
		return "Los datos enviados son iguales al registro que ya existe con ese id."
	}
	if body.CreatedAt == nil {
		diffs = append(diffs, fmt.Sprintf("created_at: no enviado; el existente tiene %s", rfc(existing.CreatedAt)))
	}
	return "Ya existe un registro con ese id. Diferencias respecto al envío: " + strings.Join(diffs, "; ") + "."
}

func writeMainJSONResponse(w http.ResponseWriter, code int, resp model.Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *HandleMain) HandleNewMain(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var body mainCreateBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeMainJSONResponse(w, http.StatusBadRequest, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    "JSON inválido: " + err.Error(),
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Items:     nil,
			})
			return
		}

		category := strings.TrimSpace(body.Category)
		subtype := strings.TrimSpace(body.Subtype)
		if category == "" || subtype == "" {
			writeMainJSONResponse(w, http.StatusBadRequest, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    "category y subtype son obligatorios y no pueden estar vacíos.",
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Items:     nil,
			})
			return
		}

		m := model.Maindb{
			Category: category,
			Subtype:  subtype,
		}
		if strings.TrimSpace(body.ID) != "" {
			m.ID = strings.TrimSpace(body.ID)
		}
		if body.CreatedAt != nil {
			m.CreatedAt = *body.CreatedAt
		}
		if body.Data != nil {
			m.Data = *body.Data
		}
		if body.Available != nil {
			m.Available = *body.Available
		} else {
			m.Available = true
		}

		idProvided := m.ID != ""
		m.ApplyDefaults()

		if idProvided {
			existing, err := h.Service.GetByID(m.ID)
			if err == nil {
				advice := adviceDuplicateMainCreate(&body, &m, existing)
				writeMainJSONResponse(w, http.StatusConflict, model.Response{
					Status:    "error",
					Total:     1,
					Advice:    advice,
					Timestamp: time.Now().UTC().Truncate(time.Second),
					Items:     []*model.Maindb{existing},
				})
				return
			}
			if !errors.Is(err, sql.ErrNoRows) {
				writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
					Status:    "error",
					Total:     0,
					Advice:    err.Error(),
					Timestamp: time.Now().UTC().Truncate(time.Second),
					Items:     nil,
				})
				return
			}
		}

		if err := h.Service.Create(&m); err != nil {
			writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    err.Error(),
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Items:     nil,
			})
			return
		}

		writeMainJSONResponse(w, http.StatusCreated, model.Response{
			Status:    "success",
			Total:     1,
			Advice:    "Registro creado correctamente.",
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     []*model.Maindb{&m},
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

}

func (h *HandleMain) HandleMainByID(w http.ResponseWriter, r *http.Request) {
	// Espera paths tipo: /api/main/{id}
	id := strings.TrimPrefix(r.URL.Path, "/api/main/")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		m, err := h.Service.GetByID(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	case http.MethodPut:
		var m model.Maindb
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.ID = id
		m.ApplyDefaults()
		if err := h.Service.Update(&m); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}