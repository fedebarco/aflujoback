package transport

import (
	"aflujo/model"
	"aflujo/sevice"
	"aflujo/store"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (h *HandleMain) authenticatedClient(w http.ResponseWriter, r *http.Request) (*model.Client, bool) {
	token := strings.TrimSpace(r.Header.Get("token"))
	now := time.Now().UTC().Truncate(time.Second)
	if token == "" {
		writeMainJSONResponse(w, http.StatusUnauthorized, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    "Falta el header token.",
			Timestamp: now,
			Items:     nil,
		})
		return nil, false
	}
	c, err := h.Service.GetClientByTokenHash(store.TokenHash(token))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMainJSONResponse(w, http.StatusUnauthorized, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    "Token inválido.",
				Timestamp: now,
				Items:     nil,
			})
			return nil, false
		}
		writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    err.Error(),
			Timestamp: now,
			Items:     nil,
		})
		return nil, false
	}
	return c, true
}

func (h *HandleMain) authenticatedAdmin(w http.ResponseWriter, r *http.Request) bool {
	user := strings.TrimSpace(r.Header.Get("user"))
	token := strings.TrimSpace(r.Header.Get("token"))
	if user == "" || token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "Faltan headers de admin: user y token."})
		return false
	}
	if !h.Service.IsAdminCredentials(user, token) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "Credenciales de admin inválidas."})
		return false
	}
	return true
}

// HandleGetAllMains lista registros maindb con el flag synced del cliente autenticado.
// @Summary Listar maindb
// @Description Filtros opcionales combinables (query). Requiere header token.
// @Tags maindb
// @Accept json
// @Produce json
// @Security TokenAuth
// @Param fromdate query string false "Desde fecha (YYYY-MM-DD UTC)"
// @Param category query string false "Categorías separadas por coma"
// @Param avariable query string false "Filtrar por available: true, false, 1 o 0"
// @Param max query int false "Máximo de filas (1-1000)"
// @Param ord query string false "Orden por created_at" Enums(asc,desc)
// @Success 200 {object} model.Response
// @Failure 401 {object} model.Response
// @Failure 500 {string} string "texto plano"
// @Router /api/main [get]
func (h *HandleMain) HandleGetAllMains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		client, ok := h.authenticatedClient(w, r)
		if !ok {
			return
		}

		q := r.URL.Query()

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

		allowedCats, err := h.Service.FilterReadableCategories(client.ID, cats)
		if err != nil {
			var denied *service.PermissionDeniedError
			if errors.As(err, &denied) {
				writePermissionDeniedResponse(w, denied.Advice)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		list, err := h.Service.GetAllFilteredForClient(client.ID, fromDate, allowedCats, avail, max, ord)
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

// HandleGetCategoryCounts lista categorías y cuántas veces aparecen en maindb.
// @Summary Contar categorías de maindb
// @Description Endpoint público. Permite filtrar opcionalmente por available con query avariable=true|false|1|0.
// @Tags maindb
// @Produce json
// @Param avariable query string false "Filtrar por available: true, false, 1 o 0"
// @Success 200 {object} model.CategoryCountResponse
// @Failure 400 {object} model.ErrorJSON
// @Failure 500 {object} model.ErrorJSON
// @Router /api/main/categories [get]
func (h *HandleMain) HandleGetCategoryCounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var avail *bool
	if v := strings.TrimSpace(r.URL.Query().Get("avariable")); v != "" {
		var b bool
		switch v {
		case "1":
			b = true
		case "0":
			b = false
		default:
			parsed, err := strconv.ParseBool(v)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "invalid avariable (expected true/false/1/0)"})
				return
			}
			b = parsed
		}
		avail = &b
	}

	items, err := h.Service.GetCategoryCounts(avail)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: err.Error()})
		return
	}

	resp := model.CategoryCountResponse{
		Status:    "success",
		Total:     len(items),
		Advice:    "Se recuperaron correctamente los conteos por categoría.",
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Items:     items,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type mainCreateBody struct {
	ID        string     `json:"id"`
	CreatedAt *time.Time `json:"created_at"`
	Category  string     `json:"category"`
	Subtype   string     `json:"subtype"`
	Data      *string    `json:"data"`
	Available *bool      `json:"available"`
}

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

func writePermissionDeniedResponse(w http.ResponseWriter, advice string) {
	writeMainJSONResponse(w, http.StatusForbidden, model.Response{
		Status:    "error",
		Total:     0,
		Advice:    advice,
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Items:     nil,
	})
}

// HandleNewMain crea un registro maindb y actualiza el estado de sync (true para el autor, false para el resto).
// @Summary Crear maindb
// @Tags maindb
// @Accept json
// @Produce json
// @Security TokenAuth
// @Param body body mainCreateBody true "category y subtype obligatorios"
// @Success 201 {object} model.Response
// @Failure 400 {object} model.Response
// @Failure 401 {object} model.Response
// @Failure 409 {object} model.Response
// @Failure 500 {object} model.Response
// @Router /api/newmain [post]
func (h *HandleMain) HandleNewMain(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		client, ok := h.authenticatedClient(w, r)
		if !ok {
			return
		}

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

		if err := h.Service.ValidateCategoryAccess(client.ID, "create", m.Category); err != nil {
			var denied *service.PermissionDeniedError
			if errors.As(err, &denied) {
				writePermissionDeniedResponse(w, denied.Advice)
				return
			}
			writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    err.Error(),
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Items:     nil,
			})
			return
		}

		idProvided := m.ID != ""
		m.ApplyDefaults()

		if idProvided {
			existing, err := h.Service.GetByID(m.ID)
			if err == nil {
				advice := adviceDuplicateMainCreate(&body, &m, existing)
				existingSync, errSync := h.Service.GetByIDForClient(m.ID, client.ID)
				if errSync != nil {
					writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
						Status:    "error",
						Total:     0,
						Advice:    errSync.Error(),
						Timestamp: time.Now().UTC().Truncate(time.Second),
						Items:     nil,
					})
					return
				}
				writeMainJSONResponse(w, http.StatusConflict, model.Response{
					Status:    "error",
					Total:     1,
					Advice:    advice,
					Timestamp: time.Now().UTC().Truncate(time.Second),
					Items:     []*model.MaindbWithSync{existingSync},
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

		if err := h.Service.CreateMainWithSync(&m, client.ID); err != nil {
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
			Items:     []*model.MaindbWithSync{{Maindb: m, Synced: true}},
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// HandleGetMainByID obtiene un maindb por id con synced del cliente.
// @Summary Obtener maindb por id
// @Tags maindb
// @Produce json
// @Security TokenAuth
// @Param id path string true "ID del registro"
// @Success 200 {object} model.Response
// @Failure 400 {object} model.Response
// @Failure 401 {object} model.Response
// @Failure 404 {object} model.Response
// @Failure 500 {object} model.Response
// @Router /api/main/{id} [get]
func (h *HandleMain) HandleGetMainByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeMainJSONResponse(w, http.StatusBadRequest, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    "Falta el id en la URL.",
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     nil,
		})
		return
	}

	client, ok := h.authenticatedClient(w, r)
	if !ok {
		return
	}

	raw, err := h.Service.GetByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMainJSONResponse(w, http.StatusNotFound, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    "No existe un registro con el id indicado.",
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Items:     nil,
			})
			return
		}
		writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    err.Error(),
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     nil,
		})
		return
	}
	if err := h.Service.ValidateCategoryAccess(client.ID, "read", raw.Category); err != nil {
		var denied *service.PermissionDeniedError
		if errors.As(err, &denied) {
			writePermissionDeniedResponse(w, denied.Advice)
			return
		}
		writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    err.Error(),
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     nil,
		})
		return
	}

	m, err := h.Service.GetByIDForClient(id, client.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMainJSONResponse(w, http.StatusNotFound, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    "No existe un registro con el id indicado.",
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Items:     nil,
			})
			return
		}
		writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    err.Error(),
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     nil,
		})
		return
	}
	writeMainJSONResponse(w, http.StatusOK, model.Response{
		Status:    "success",
		Total:     1,
		Advice:    "Registro recuperado correctamente.",
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Items:     []*model.MaindbWithSync{m},
	})
}

// HandlePutMainByID pone available en false y actualiza sync (true quien llama, false el resto).
// @Summary Deshabilitar maindb
// @Description Pone available a false. Si ya era false, responde 200 con mensaje informativo.
// @Tags maindb
// @Produce json
// @Security TokenAuth
// @Param id path string true "ID del registro"
// @Success 200 {object} model.Response
// @Failure 401 {object} model.Response
// @Failure 404 {object} model.Response
// @Failure 500 {object} model.Response
// @Router /api/main/{id} [put]
func (h *HandleMain) HandlePutMainByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeMainJSONResponse(w, http.StatusBadRequest, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    "Falta el id en la URL.",
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     nil,
		})
		return
	}

	client, ok := h.authenticatedClient(w, r)
	if !ok {
		return
	}

	raw, err := h.Service.GetByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMainJSONResponse(w, http.StatusNotFound, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    "No existe un registro con el id indicado.",
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Items:     nil,
			})
			return
		}
		writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    err.Error(),
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     nil,
		})
		return
	}
	if err := h.Service.ValidateCategoryAccess(client.ID, "write", raw.Category); err != nil {
		var denied *service.PermissionDeniedError
		if errors.As(err, &denied) {
			writePermissionDeniedResponse(w, denied.Advice)
			return
		}
		writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    err.Error(),
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     nil,
		})
		return
	}
	if !raw.Available {
		mws, err := h.Service.GetByIDForClient(id, client.ID)
		if err != nil {
			writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
				Status:    "error",
				Total:     0,
				Advice:    err.Error(),
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Items:     nil,
			})
			return
		}
		writeMainJSONResponse(w, http.StatusOK, model.Response{
			Status:    "success",
			Total:     1,
			Advice:    "El registro ya se había actualizado (available ya era false).",
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     []*model.MaindbWithSync{mws},
		})
		return
	}
	m, err := h.Service.UpdateMainAvailableFalseWithSync(id, client.ID)
	if err != nil {
		writeMainJSONResponse(w, http.StatusInternalServerError, model.Response{
			Status:    "error",
			Total:     0,
			Advice:    err.Error(),
			Timestamp: time.Now().UTC().Truncate(time.Second),
			Items:     nil,
		})
		return
	}
	writeMainJSONResponse(w, http.StatusOK, model.Response{
		Status:    "success",
		Total:     1,
		Advice:    "Registro deshabilitado correctamente (available pasó a false).",
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Items:     []*model.MaindbWithSync{m},
	})
}

type createClientBody struct {
	Name string `json:"name"`
}

// HandleCreateClient registra un cliente nuevo (sin token). Devuelve el token en claro una sola vez.
// @Summary Registrar cliente
// @Description Requiere headers admin (`user` y `token`). El campo token de la respuesta solo se muestra en este alta.
// @Tags clients
// @Accept json
// @Produce json
// @Param user header string true "Usuario admin (MAIN_USER)"
// @Param token header string true "Token admin (MAIN_TOKEN)"
// @Param body body createClientBody false "Nombre opcional"
// @Success 201 {object} model.ClientCreatedResponse
// @Failure 401 {object} model.ErrorJSON
// @Failure 400 {object} model.ErrorJSON
// @Failure 500 {object} model.ErrorJSON
// @Router /api/clients [post]
func (h *HandleMain) HandleCreateClient(w http.ResponseWriter, r *http.Request) {
	if !h.authenticatedAdmin(w, r) {
		return
	}
	var body createClientBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}
	out, err := h.Service.RegisterClient(body.Name)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(out)
}

// HandleListClients lista clientes (sin token_hash).
// @Summary Listar clientes
// @Description Requiere headers admin (`user` y `token`).
// @Tags clients
// @Produce json
// @Param user header string true "Usuario admin (MAIN_USER)"
// @Param token header string true "Token admin (MAIN_TOKEN)"
// @Success 200 {array} model.Client
// @Failure 401 {object} model.ErrorJSON
// @Failure 500 {object} model.ErrorJSON
// @Router /api/clients [get]
func (h *HandleMain) HandleListClients(w http.ResponseWriter, r *http.Request) {
	if !h.authenticatedAdmin(w, r) {
		return
	}
	list, err := h.Service.ListClients()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(list)
}

type updateClientBody struct {
	Name        *string `json:"name"`
	RotateToken bool    `json:"rotate_token"`
}

// HandleUpdateClient actualiza nombre y/o rota token de un cliente.
// @Summary Editar cliente
// @Description Requiere headers admin (`user` y `token`). No permite editar el usuario admin.
// @Tags clients
// @Accept json
// @Produce json
// @Param user header string true "Usuario admin (MAIN_USER)"
// @Param token header string true "Token admin (MAIN_TOKEN)"
// @Param id path string true "ID del cliente"
// @Param body body updateClientBody true "name opcional, rotate_token opcional"
// @Success 200 {object} model.ClientUpdatedResponse
// @Failure 400 {object} model.ErrorJSON
// @Failure 401 {object} model.ErrorJSON
// @Failure 404 {object} model.ErrorJSON
// @Failure 500 {object} model.ErrorJSON
// @Router /api/clients/{id} [put]
func (h *HandleMain) HandleUpdateClient(w http.ResponseWriter, r *http.Request) {
	if !h.authenticatedAdmin(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "Falta el id del cliente."})
		return
	}

	var body updateClientBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "JSON inválido: " + err.Error()})
		return
	}

	out, err := h.Service.UpdateClient(id, body.Name, body.RotateToken)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case errors.Is(err, sql.ErrNoRows):
			w.WriteHeader(http.StatusNotFound)
		case errors.Is(err, service.ErrAdminProtected):
			w.WriteHeader(http.StatusBadRequest)
		default:
			if strings.Contains(strings.ToLower(err.Error()), "no changes") ||
				strings.Contains(strings.ToLower(err.Error()), "cannot be empty") {
				w.WriteHeader(http.StatusBadRequest)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

// HandleGetClientPermissions devuelve permisos configurados de un cliente.
// @Summary Obtener permisos de cliente
// @Description Requiere headers admin (`user` y `token`).
// @Tags clients
// @Produce json
// @Param user header string true "Usuario admin (MAIN_USER)"
// @Param token header string true "Token admin (MAIN_TOKEN)"
// @Param id path string true "ID del cliente"
// @Success 200 {object} model.ClientPermissions
// @Failure 400 {object} model.ErrorJSON
// @Failure 401 {object} model.ErrorJSON
// @Failure 404 {object} model.ErrorJSON
// @Failure 500 {object} model.ErrorJSON
// @Router /api/clients/{id}/permissions [get]
func (h *HandleMain) HandleGetClientPermissions(w http.ResponseWriter, r *http.Request) {
	if !h.authenticatedAdmin(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "Falta el id del cliente."})
		return
	}
	if _, err := h.Service.GetClientByID(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: err.Error()})
		return
	}
	out, err := h.Service.GetClientPermissions(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

// HandleUpsertClientPermissions reemplaza permisos de un cliente.
// @Summary Configurar permisos de cliente
// @Description Requiere headers admin (`user` y `token`). Reemplaza la configuración completa del cliente.
// @Tags clients
// @Accept json
// @Produce json
// @Param user header string true "Usuario admin (MAIN_USER)"
// @Param token header string true "Token admin (MAIN_TOKEN)"
// @Param id path string true "ID del cliente"
// @Param body body model.UpsertClientPermissionsBody true "Permisos por categoría"
// @Success 200 {object} model.ClientPermissions
// @Failure 400 {object} model.ErrorJSON
// @Failure 401 {object} model.ErrorJSON
// @Failure 404 {object} model.ErrorJSON
// @Failure 500 {object} model.ErrorJSON
// @Router /api/clients/{id}/permissions [put]
func (h *HandleMain) HandleUpsertClientPermissions(w http.ResponseWriter, r *http.Request) {
	if !h.authenticatedAdmin(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "Falta el id del cliente."})
		return
	}
	if _, err := h.Service.GetClientByID(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: err.Error()})
		return
	}

	var body model.UpsertClientPermissionsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "JSON inválido: " + err.Error()})
		return
	}
	out, err := h.Service.UpsertClientPermissions(id, body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(strings.ToLower(err.Error()), "max_create_categories") ||
			strings.Contains(strings.ToLower(err.Error()), "exceeds") {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

// HandleDeleteClient elimina un cliente.
// @Summary Eliminar cliente
// @Description Requiere headers admin (`user` y `token`). No permite eliminar el usuario admin.
// @Tags clients
// @Produce json
// @Param user header string true "Usuario admin (MAIN_USER)"
// @Param token header string true "Token admin (MAIN_TOKEN)"
// @Param id path string true "ID del cliente"
// @Success 204 {string} string "No Content"
// @Failure 400 {object} model.ErrorJSON
// @Failure 401 {object} model.ErrorJSON
// @Failure 404 {object} model.ErrorJSON
// @Failure 500 {object} model.ErrorJSON
// @Router /api/clients/{id} [delete]
func (h *HandleMain) HandleDeleteClient(w http.ResponseWriter, r *http.Request) {
	if !h.authenticatedAdmin(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: "Falta el id del cliente."})
		return
	}
	err := h.Service.DeleteClient(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case errors.Is(err, sql.ErrNoRows):
			w.WriteHeader(http.StatusNotFound)
		case errors.Is(err, service.ErrAdminProtected):
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = json.NewEncoder(w).Encode(model.ErrorJSON{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
