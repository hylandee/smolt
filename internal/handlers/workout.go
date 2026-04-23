package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"stronglifts/internal/auth"
	"stronglifts/internal/db"
	"stronglifts/internal/templates"
	"stronglifts/internal/workout"

	"github.com/go-chi/chi/v5"
)

type WorkoutHandlers struct {
	progression *workout.ProgressionService
	authService *auth.AuthService
}

type trackingSetInput struct {
	SetNumber  int     `json:"setNumber"`
	ActualReps int     `json:"actualReps"`
	Weight     float64 `json:"weight"`
	Completed  bool    `json:"completed"`
}

func NewWorkoutHandlers(database *db.DB) *WorkoutHandlers {
	return &WorkoutHandlers{
		progression: workout.NewProgressionService(database.Conn()),
		authService: auth.NewAuthService(database.Conn()),
	}
}

func (h *WorkoutHandlers) weightUnitForUser(r *http.Request, userID int) (string, error) {
	unitPref, err := h.authService.GetUnitPref(r.Context(), userID)
	if err != nil {
		return "", err
	}
	if unitPref == auth.UnitPrefMetric {
		return "kg", nil
	}
	return "lb", nil
}

func (h *WorkoutHandlers) distanceUnitForUser(r *http.Request, userID int) (string, error) {
	pref, err := h.authService.GetDistanceUnitPref(r.Context(), userID)
	if err != nil {
		return "", err
	}
	if pref == auth.DistanceUnitKM {
		return "km", nil
	}
	return "mi", nil
}

// isHTMX reports whether the request came from HTMX
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// GET /workout/next — JSON API used by tests and client-side scripts
func (h *WorkoutHandlers) NextWorkout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	program, weights, err := h.progression.NextWorkoutPlan(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to get next workout", http.StatusInternalServerError)
		return
	}

	exercises := make([]map[string]any, 0, len(program.Exercises))
	for _, ex := range program.Exercises {
		exercises = append(exercises, map[string]any{
			"name":   ex.Name,
			"weight": weights[ex.Name],
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"program":   program.Name,
		"exercises": exercises,
	})
}

// GET /workout/{id} — renders the active workout page
func (h *WorkoutHandlers) WorkoutPage(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	session, err := h.progression.GetSession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "Failed to load session", http.StatusInternalServerError)
		return
	}
	weightUnit, err := h.weightUnitForUser(r, user.UserID)
	if err != nil {
		http.Error(w, "Failed to load unit preference", http.StatusInternalServerError)
		return
	}
	distanceUnit, err := h.distanceUnitForUser(r, user.UserID)
	if err != nil {
		http.Error(w, "Failed to load distance unit preference", http.StatusInternalServerError)
		return
	}
	workouts, err := h.progression.ListStandaloneWorkouts(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load standalone workouts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	templates.Render(w, "workout.html", map[string]any{
		"User":               user,
		"Session":            session,
		"WeightUnit":         weightUnit,
		"DistanceUnit":       distanceUnit,
		"StandaloneWorkouts": workouts,
	})
}

// POST /workout/start
// HTMX: returns full workout page HTML with HX-Push-Url header
// JSON:  returns {"sessionId": N} with 201
func (h *WorkoutHandlers) StartWorkout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	workoutType := "next"
	programName := ""
	standaloneID := int64(0)
	isJSONRequest := false

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		isJSONRequest = true
		var body struct {
			WorkoutType  string `json:"workoutType"`
			ProgramName  string `json:"programName"`
			StandaloneID int64  `json:"standaloneId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.WorkoutType != "" {
			workoutType = strings.ToLower(strings.TrimSpace(body.WorkoutType))
		}
		programName = strings.ToUpper(strings.TrimSpace(body.ProgramName))
		standaloneID = body.StandaloneID
	} else {
		if err := r.ParseForm(); err == nil {
			if v := strings.ToLower(strings.TrimSpace(r.FormValue("workout_type"))); v != "" {
				workoutType = v
			}
			programName = strings.ToUpper(strings.TrimSpace(r.FormValue("program_name")))
			if rawID := strings.TrimSpace(r.FormValue("standalone_id")); rawID != "" {
				if parsedID, err := strconv.ParseInt(rawID, 10, 64); err == nil {
					standaloneID = parsedID
				}
			}
		}
	}

	var sessionID int64
	var err error

	switch workoutType {
	case "program":
		if programName == "" {
			http.Error(w, "Missing program name", http.StatusBadRequest)
			return
		}
		program, weights, planErr := h.progression.ProgramPlan(r.Context(), user.UserID, programName)
		if planErr != nil {
			http.Error(w, "Invalid program selection", http.StatusBadRequest)
			return
		}
		sessionID, err = h.progression.StartSession(r.Context(), user.UserID, program, weights)
	case "standalone":
		if standaloneID <= 0 {
			http.Error(w, "Missing standalone workout", http.StatusBadRequest)
			return
		}
		sessionID, err = h.progression.StartSessionFromStandalone(r.Context(), user.UserID, standaloneID)
	default:
		program, weights, planErr := h.progression.NextWorkoutPlan(r.Context(), user.UserID)
		if planErr != nil {
			http.Error(w, "Failed to plan workout", http.StatusInternalServerError)
			return
		}
		sessionID, err = h.progression.StartSession(r.Context(), user.UserID, program, weights)
	}
	if err != nil {
		http.Error(w, "Failed to start session", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		w.Header().Set("HX-Redirect", fmt.Sprintf("/workout/%d", sessionID))
		w.WriteHeader(http.StatusOK)
		return
	}

	if !isJSONRequest {
		http.Redirect(w, r, fmt.Sprintf("/workout/%d", sessionID), http.StatusFound)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"sessionId": sessionID})
}

// StandaloneEditor handles GET /workout/standalone/new
func (h *WorkoutHandlers) StandaloneEditor(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	weightUnit, err := h.weightUnitForUser(r, user.UserID)
	if err != nil {
		http.Error(w, "Failed to load unit preference", http.StatusInternalServerError)
		return
	}
	distanceUnit, err := h.distanceUnitForUser(r, user.UserID)
	if err != nil {
		http.Error(w, "Failed to load distance unit preference", http.StatusInternalServerError)
		return
	}
	h.renderStandaloneEditor(w, user, weightUnit, distanceUnit, nil, "", "/workout/standalone", "Save Standalone Workout")
}

// EditStandaloneWorkout handles GET /workout/standalone/{id}/edit
func (h *WorkoutHandlers) EditStandaloneWorkout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	workoutID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid workout ID", http.StatusBadRequest)
		return
	}
	detail, err := h.progression.GetStandaloneWorkout(r.Context(), user.UserID, workoutID)
	if err != nil {
		http.Error(w, "Standalone workout not found", http.StatusNotFound)
		return
	}
	weightUnit, err := h.weightUnitForUser(r, user.UserID)
	if err != nil {
		http.Error(w, "Failed to load unit preference", http.StatusInternalServerError)
		return
	}
	distanceUnit, err := h.distanceUnitForUser(r, user.UserID)
	if err != nil {
		http.Error(w, "Failed to load distance unit preference", http.StatusInternalServerError)
		return
	}
	h.renderStandaloneEditor(w, user, weightUnit, distanceUnit, detail, "", fmt.Sprintf("/workout/standalone/%d", workoutID), "Update Standalone Workout")
}

// CreateStandaloneWorkout handles POST /workout/standalone
func (h *WorkoutHandlers) CreateStandaloneWorkout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	notes := strings.TrimSpace(r.FormValue("notes"))
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		h.renderStandaloneEditorError(w, r, user, nil, "Please provide a workout name.", "/workout/standalone", "Save Standalone Workout")
		return
	}
	items, ok := h.parseStandaloneItems(w, r, user, nil, "/workout/standalone", "Save Standalone Workout")
	if !ok {
		return
	}

	if _, err := h.progression.SaveStandaloneWorkout(r.Context(), user.UserID, title, notes, items); err != nil {
		h.renderStandaloneEditorError(w, r, user, nil, "Failed to save standalone workout", "/workout/standalone", "Save Standalone Workout")
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// UpdateStandaloneWorkout handles POST /workout/standalone/{id}
func (h *WorkoutHandlers) UpdateStandaloneWorkout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	workoutID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid workout ID", http.StatusBadRequest)
		return
	}
	detail, err := h.progression.GetStandaloneWorkout(r.Context(), user.UserID, workoutID)
	if err != nil {
		http.Error(w, "Standalone workout not found", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	notes := strings.TrimSpace(r.FormValue("notes"))
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		h.renderStandaloneEditorError(w, r, user, detail, "Please provide a workout name.", fmt.Sprintf("/workout/standalone/%d", workoutID), "Update Standalone Workout")
		return
	}
	items, ok := h.parseStandaloneItems(w, r, user, detail, fmt.Sprintf("/workout/standalone/%d", workoutID), "Update Standalone Workout")
	if !ok {
		return
	}

	if err := h.progression.UpdateStandaloneWorkout(r.Context(), user.UserID, workoutID, title, notes, items); err != nil {
		h.renderStandaloneEditorError(w, r, user, detail, "Failed to update standalone workout", fmt.Sprintf("/workout/standalone/%d", workoutID), "Update Standalone Workout")
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// DeleteStandaloneWorkout handles POST /workout/standalone/{id}/delete
func (h *WorkoutHandlers) DeleteStandaloneWorkout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	workoutID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid workout ID", http.StatusBadRequest)
		return
	}
	if err := h.progression.DeleteStandaloneWorkout(r.Context(), user.UserID, workoutID); err != nil {
		http.Error(w, "Failed to delete standalone workout", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// DeleteAllStandaloneWorkouts handles POST /workout/standalone/delete-all
func (h *WorkoutHandlers) DeleteAllStandaloneWorkouts(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	if _, err := h.progression.DeleteAllStandaloneWorkouts(r.Context(), user.UserID); err != nil {
		http.Error(w, "Failed to delete standalone workouts", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *WorkoutHandlers) parseStandaloneItems(w http.ResponseWriter, r *http.Request, user *auth.UserSession, existing *workout.StandaloneWorkoutDetail, formAction, submitLabel string) ([]workout.StandaloneItemInput, bool) {
	types := r.Form["exercise_type"]
	names := r.Form["exercise_name"]
	sets := r.Form["sets"]
	reps := r.Form["target_reps"]
	weights := r.Form["weight"]
	setSchemes := r.Form["set_scheme_json"]
	times := r.Form["time_minutes"]
	distances := r.Form["distance_miles"]

	if len(types) == 0 {
		h.renderStandaloneEditorError(w, r, user, existing, "Add at least one exercise.", formAction, submitLabel)
		return nil, false
	}

	items := make([]workout.StandaloneItemInput, 0, len(types))
	for i, rawType := range types {
		exerciseType := strings.ToLower(strings.TrimSpace(rawType))
		exerciseName := strings.TrimSpace(valueAt(names, i))
		if exerciseName == "" {
			exerciseName = defaultExerciseName(exerciseType)
		}

		switch exerciseType {
		case workout.StandaloneTypeStrength:
			var scheme []workout.StandaloneStrengthSetInput
			rawScheme := strings.TrimSpace(valueAt(setSchemes, i))
			if rawScheme != "" {
				if err := json.Unmarshal([]byte(rawScheme), &scheme); err != nil {
					h.renderStandaloneEditorError(w, r, user, existing, "Invalid strength set scheme.", formAction, submitLabel)
					return nil, false
				}
			}
			setCount := parsePositiveIntWithDefault(valueAt(sets, i), 5)
			targetReps := parsePositiveIntWithDefault(valueAt(reps, i), 5)
			weight := parseNonNegativeFloatWithDefault(valueAt(weights, i), 0)
			if len(scheme) > 0 {
				setCount = len(scheme)
				targetReps = scheme[0].TargetReps
				weight = scheme[0].Weight
			}
			items = append(items, workout.StandaloneItemInput{
				ExerciseName: exerciseName,
				ExerciseType: exerciseType,
				Sets:         setCount,
				TargetReps:   targetReps,
				Weight:       weight,
				SetScheme:    scheme,
			})
		case workout.StandaloneTypeTreadmill, workout.StandaloneTypeStaircase, workout.StandaloneTypeBike, workout.StandaloneTypeElliptical:
			timeMinutes := parsePositiveIntWithDefault(valueAt(times, i), 10)
			distanceMiles := 0.0
			if exerciseType == workout.StandaloneTypeTreadmill || exerciseType == workout.StandaloneTypeBike {
				distanceMiles = parsePositiveFloatWithDefault(valueAt(distances, i), 1)
			}
			items = append(items, workout.StandaloneItemInput{
				ExerciseName:  exerciseName,
				ExerciseType:  exerciseType,
				TimeMinutes:   timeMinutes,
				DistanceMiles: distanceMiles,
			})
		default:
			h.renderStandaloneEditorError(w, r, user, existing, "Unsupported exercise type selected.", formAction, submitLabel)
			return nil, false
		}
	}

	return items, true
}

func (h *WorkoutHandlers) renderStandaloneEditor(w http.ResponseWriter, user *auth.UserSession, weightUnit, distanceUnit string, existing *workout.StandaloneWorkoutDetail, errorMsg, formAction, submitLabel string) {
	var initialJSON string
	if existing != nil {
		if data, err := json.Marshal(existing); err == nil {
			initialJSON = string(data)
		}
	}
	w.Header().Set("Content-Type", "text/html")
	templates.Render(w, "standalone_editor.html", map[string]any{
		"User":                user,
		"Error":               errorMsg,
		"WeightUnit":          weightUnit,
		"DistanceUnit":        distanceUnit,
		"FormAction":          formAction,
		"SubmitLabel":         submitLabel,
		"ExistingWorkout":     existing,
		"ExistingWorkoutJSON": initialJSON,
		"IsEdit":              existing != nil,
	})
}

func (h *WorkoutHandlers) renderStandaloneEditorError(w http.ResponseWriter, r *http.Request, user *auth.UserSession, existing *workout.StandaloneWorkoutDetail, msg, formAction, submitLabel string) {
	weightUnit, err := h.weightUnitForUser(r, user.UserID)
	if err != nil {
		weightUnit = "lb"
	}
	distanceUnit, err := h.distanceUnitForUser(r, user.UserID)
	if err != nil {
		distanceUnit = "mi"
	}
	h.renderStandaloneEditor(w, user, weightUnit, distanceUnit, existing, msg, formAction, submitLabel)
}

// POST /workout/{id}/set/{n}/complete
// HTMX: returns updated bubble HTML
// JSON:  returns {"ok": true}
func (h *WorkoutHandlers) CompleteSet(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}
	setNum, err := strconv.Atoi(chi.URLParam(r, "n"))
	if err != nil || setNum < 1 {
		http.Error(w, "Invalid set number", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	var actualReps int
	var weight float64

	ct := r.Header.Get("Content-Type")
	if ct == "application/json" {
		var body struct {
			ActualReps int     `json:"actualReps"`
			Weight     float64 `json:"weight"`
			Reps       int     `json:"reps"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		actualReps = body.ActualReps
		if actualReps == 0 {
			actualReps = body.Reps
		}
		weight = body.Weight
	} else {
		r.ParseForm()
		actualReps, _ = strconv.Atoi(r.FormValue("reps"))
		weight, _ = strconv.ParseFloat(r.FormValue("weight"), 64)
	}
	if actualReps == 0 {
		actualReps = 5
	}

	if actualReps < 0 {
		http.Error(w, "reps must be >= 0", http.StatusBadRequest)
		return
	}

	if err := h.progression.CompleteSet(r.Context(), sessionID, setNum, actualReps, weight); err != nil {
		http.Error(w, "Failed to complete set", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		weightUnit, err := h.weightUnitForUser(r, user.UserID)
		if err != nil {
			http.Error(w, "Failed to load unit preference", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w,
			`<div id="set-%d" class="set-bubble complete" title="Set %d · %.1f %s">%d</div>`,
			setNum, setNum, weight, weightUnit, setNum,
		)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /workout/{id}/finish
// HTMX: returns dashboard HTML with HX-Push-Url header
// JSON:  returns {"progressUpdates": [...]}
func (h *WorkoutHandlers) FinishWorkout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	updatesPayload, parseErr := parseTrackingSetUpdates(r)
	if parseErr != nil {
		http.Error(w, "Invalid workout payload", http.StatusBadRequest)
		return
	}
	if len(updatesPayload) > 0 {
		updates := make([]workout.SetUpdate, 0, len(updatesPayload))
		for _, u := range updatesPayload {
			updates = append(updates, workout.SetUpdate{
				SetNumber:  u.SetNumber,
				ActualReps: u.ActualReps,
				Weight:     u.Weight,
				Completed:  u.Completed,
			})
		}
		if err := h.progression.ApplySetUpdates(r.Context(), sessionID, updates); err != nil {
			http.Error(w, "Failed to save workout data", http.StatusInternalServerError)
			return
		}
	}

	updates, err := h.progression.FinishSession(r.Context(), sessionID, user.UserID)
	if err != nil {
		http.Error(w, "Failed to finish session", http.StatusInternalServerError)
		return
	}
	summary, _ := h.progression.SessionFinishSummary(r.Context(), user.UserID, sessionID)

	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", "/")
		h.renderDashboard(w, r, user)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId":       sessionID,
		"progressUpdates": updates,
		"finishSummary":   summary,
	})
}

// SaveTracking handles POST /workout/{id}/save with bulk set updates.
func (h *WorkoutHandlers) SaveTracking(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	updatesPayload, parseErr := parseTrackingSetUpdates(r)
	if parseErr != nil {
		http.Error(w, "Invalid workout payload", http.StatusBadRequest)
		return
	}

	updates := make([]workout.SetUpdate, 0, len(updatesPayload))
	for _, u := range updatesPayload {
		updates = append(updates, workout.SetUpdate{
			SetNumber:  u.SetNumber,
			ActualReps: u.ActualReps,
			Weight:     u.Weight,
			Completed:  u.Completed,
		})
	}

	if err := h.progression.ApplySetUpdates(r.Context(), sessionID, updates); err != nil {
		http.Error(w, "Failed to save workout data", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// AddExercise handles POST /workout/{id}/exercise/add
func (h *WorkoutHandlers) AddExercise(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	exerciseType := strings.TrimSpace(r.FormValue("exercise_type"))
	exerciseName := strings.TrimSpace(r.FormValue("exercise_name"))
	sets := parsePositiveIntWithDefault(r.FormValue("sets"), 1)
	targetReps := parsePositiveIntWithDefault(r.FormValue("target_reps"), 5)
	weight := parseNonNegativeFloatWithDefault(r.FormValue("weight"), 0)

	if exerciseType == "cardio" {
		switch exerciseName {
		case "Treadmill":
			exerciseType = workout.StandaloneTypeTreadmill
		case "Exercise Bike":
			exerciseType = workout.StandaloneTypeBike
		case "Staircase":
			exerciseType = workout.StandaloneTypeStaircase
		case "Elliptical":
			exerciseType = workout.StandaloneTypeElliptical
		default:
			exerciseType = workout.StandaloneTypeElliptical
		}
	}

	if exerciseType == workout.StandaloneTypeTreadmill || exerciseType == workout.StandaloneTypeBike {
		targetReps = parsePositiveIntWithDefault(r.FormValue("time_minutes"), 10)
		weight = parsePositiveFloatWithDefault(r.FormValue("distance_miles"), 1)
		sets = 1
	} else if exerciseType == workout.StandaloneTypeStaircase || exerciseType == workout.StandaloneTypeElliptical {
		targetReps = parsePositiveIntWithDefault(r.FormValue("time_minutes"), 10)
		weight = 0
		sets = 1
	}

	if err := h.progression.AddExerciseToSession(r.Context(), sessionID, exerciseType, exerciseName, sets, targetReps, weight); err != nil {
		http.Error(w, "Failed to add exercise", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/workout/%d", sessionID), http.StatusFound)
}

// AddSetToExercise handles POST /workout/{id}/exercise/{group}/set/add
func (h *WorkoutHandlers) AddSetToExercise(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	groupIndex, err := strconv.Atoi(strings.TrimSpace(chi.URLParam(r, "group")))
	if err != nil || groupIndex < 1 {
		http.Error(w, "Invalid exercise group", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	targetReps := parsePositiveIntWithDefault(r.FormValue("target_reps"), 0)
	weight := parseNonNegativeFloatWithDefault(r.FormValue("weight"), -1)

	if err := h.progression.AddSetToExerciseGroup(r.Context(), sessionID, groupIndex, targetReps, weight); err != nil {
		http.Error(w, "Failed to add set", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/workout/%d", sessionID), http.StatusFound)
}

// DeleteSet handles DELETE /workout/{id}/set/{n}
func (h *WorkoutHandlers) DeleteSet(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}
	setNum, err := strconv.Atoi(chi.URLParam(r, "n"))
	if err != nil || setNum < 1 {
		http.Error(w, "Invalid set number", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if err := h.progression.DeleteSet(r.Context(), sessionID, setNum); err != nil {
		http.Error(w, "Failed to delete set", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DeloadExercise handles POST /progress/{exercise}/deload
func (h *WorkoutHandlers) DeloadExercise(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	exerciseName := strings.TrimSpace(chi.URLParam(r, "exercise"))
	if exerciseName == "" {
		http.Error(w, "Missing exercise", http.StatusBadRequest)
		return
	}

	if err := h.progression.DeloadExercise(r.Context(), user.UserID, exerciseName); err != nil {
		http.Error(w, "Failed to deload exercise", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// ToggleSkipIncrement handles POST /progress/{exercise}/skip-increment
func (h *WorkoutHandlers) ToggleSkipIncrement(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	exerciseName := strings.TrimSpace(chi.URLParam(r, "exercise"))
	if exerciseName == "" {
		http.Error(w, "Missing exercise", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}
	skip := strings.TrimSpace(r.FormValue("skip"))
	shouldSkip := skip == "1" || strings.EqualFold(skip, "true")

	if err := h.progression.SetSkipNextIncrement(r.Context(), user.UserID, exerciseName, shouldSkip); err != nil {
		http.Error(w, "Failed to update skip increment", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// ExportBackup handles GET /backup/export.
func (h *WorkoutHandlers) ExportBackup(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	backup, err := h.progression.ExportBackup(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to export backup", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=smolt-backup.json")
	if err := json.NewEncoder(w).Encode(backup); err != nil {
		http.Error(w, "Failed to encode backup", http.StatusInternalServerError)
		return
	}
}

// ImportBackup handles POST /backup/import.
func (h *WorkoutHandlers) ImportBackup(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	raw := strings.TrimSpace(r.FormValue("backup_json"))
	if raw == "" {
		payload, _ := io.ReadAll(r.Body)
		raw = strings.TrimSpace(string(payload))
	}
	if raw == "" {
		http.Error(w, "Backup payload is required", http.StatusBadRequest)
		return
	}

	var backup workout.BackupData
	if err := json.Unmarshal([]byte(raw), &backup); err != nil {
		http.Error(w, "Invalid backup payload", http.StatusBadRequest)
		return
	}

	if err := h.progression.ImportBackup(r.Context(), user.UserID, backup); err != nil {
		http.Error(w, "Failed to import backup", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/profile?settings_saved=1", http.StatusFound)
}

// FinishOpenWorkouts marks all in-progress workouts as finished for the current user.
func (h *WorkoutHandlers) FinishOpenWorkouts(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	finishedCount, err := h.progression.FinishOpenSessions(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to finish open workouts", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		h.renderDashboard(w, r, user)
		return
	}

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusOK, map[string]any{"finished": finishedCount})
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// DeleteExercise removes a contiguous exercise block from a session.
func (h *WorkoutHandlers) DeleteExercise(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	groupIndex, err := strconv.Atoi(strings.TrimSpace(chi.URLParam(r, "group")))
	if err != nil || groupIndex < 1 {
		http.Error(w, "Invalid exercise group", http.StatusBadRequest)
		return
	}

	if err := h.progression.DeleteExerciseGroup(r.Context(), sessionID, groupIndex); err != nil {
		http.Error(w, "Failed to delete exercise", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ReorderExercises moves one exercise block to a new position.
func (h *WorkoutHandlers) ReorderExercises(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	var body struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid reorder payload", http.StatusBadRequest)
		return
	}

	if err := h.progression.ReorderExerciseGroups(r.Context(), sessionID, body.From, body.To); err != nil {
		http.Error(w, "Failed to reorder exercises", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ProgressCharts handles GET /progress/charts
func (h *WorkoutHandlers) ProgressCharts(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	strengthSeries, err := h.progression.StrengthProgressSeries(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load progress charts", http.StatusInternalServerError)
		return
	}
	cardioSeries, err := h.progression.CardioProgressSeries(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load cardio charts", http.StatusInternalServerError)
		return
	}
	seriesJSONBytes, err := json.Marshal(strengthSeries)
	if err != nil {
		http.Error(w, "Failed to serialize progress charts", http.StatusInternalServerError)
		return
	}
	cardioJSONBytes, err := json.Marshal(cardioSeries)
	if err != nil {
		http.Error(w, "Failed to serialize cardio charts", http.StatusInternalServerError)
		return
	}

	distanceUnit, err := h.distanceUnitForUser(r, user.UserID)
	if err != nil {
		distanceUnit = "mi"
	}

	w.Header().Set("Content-Type", "text/html")
	templates.Render(w, "progress_charts.html", map[string]any{
		"User":               user,
		"StrengthSeriesJSON": string(seriesJSONBytes),
		"CardioSeriesJSON":   string(cardioJSONBytes),
		"DistanceUnit":       distanceUnit,
	})
}

// DELETE /workout/{id}
// HTMX: returns dashboard HTML with HX-Push-Url header
// JSON:  returns {"deleted": true}
func (h *WorkoutHandlers) DeleteWorkout(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)

	sessionID, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	ownerID, err := h.progression.SessionOwner(r.Context(), sessionID)
	if err != nil || ownerID != user.UserID {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if err := h.progression.DeleteSession(r.Context(), sessionID); err != nil {
		http.Error(w, "Failed to delete session", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		w.Header().Set("HX-Push-Url", "/")
		h.renderDashboard(w, r, user)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *WorkoutHandlers) renderDashboard(w http.ResponseWriter, r *http.Request, user *auth.UserSession) {
	program, weights, err := h.progression.NextWorkoutPlan(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load workout plan", http.StatusInternalServerError)
		return
	}
	workoutA, weightsA, err := h.progression.ProgramPlan(r.Context(), user.UserID, "A")
	if err != nil {
		http.Error(w, "Failed to load workout A", http.StatusInternalServerError)
		return
	}
	workoutB, weightsB, err := h.progression.ProgramPlan(r.Context(), user.UserID, "B")
	if err != nil {
		http.Error(w, "Failed to load workout B", http.StatusInternalServerError)
		return
	}
	customWorkouts, err := h.progression.ListStandaloneWorkouts(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load standalone workouts", http.StatusInternalServerError)
		return
	}
	currentPage := parseDashboardPage(r)
	totalSessions, err := h.progression.CountSessions(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load session count", http.StatusInternalServerError)
		return
	}
	currentPage = clampDashboardPage(currentPage, totalSessions)
	offset := (currentPage - 1) * dashboardRecentSessionsPageSize
	recentSessions, err := h.progression.ListSessionHistoryPage(r.Context(), user.UserID, dashboardRecentSessionsPageSize, offset)
	if err != nil {
		http.Error(w, "Failed to load session history", http.StatusInternalServerError)
		return
	}
	openWorkoutCount, err := h.progression.CountOpenSessions(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load open workouts", http.StatusInternalServerError)
		return
	}

	var finishSummary *workout.FinishSummary
	if raw := strings.TrimSpace(r.URL.Query().Get("finished")); raw != "" {
		if sessionID, err := strconv.ParseInt(raw, 10, 64); err == nil {
			summary, err := h.progression.SessionFinishSummary(r.Context(), user.UserID, sessionID)
			if err == nil {
				finishSummary = summary
				distanceUnit, err := h.distanceUnitForUser(r, user.UserID)
				if err == nil && distanceUnit == "km" {
					finishSummary.CardioMiles = finishSummary.CardioMiles * 1.60934
				}
			}
		}
	}

	type ExercisePlan struct {
		Name   string
		Weight float64
	}
	type WorkoutPlan struct {
		ProgramName string
		Exercises   []ExercisePlan
	}

	plan := WorkoutPlan{ProgramName: program.Name}
	for _, ex := range program.Exercises {
		plan.Exercises = append(plan.Exercises, ExercisePlan{
			Name:   ex.Name,
			Weight: weights[ex.Name],
		})
	}

	planA := WorkoutPlan{ProgramName: workoutA.Name}
	for _, ex := range workoutA.Exercises {
		planA.Exercises = append(planA.Exercises, ExercisePlan{
			Name:   ex.Name,
			Weight: weightsA[ex.Name],
		})
	}

	planB := WorkoutPlan{ProgramName: workoutB.Name}
	for _, ex := range workoutB.Exercises {
		planB.Exercises = append(planB.Exercises, ExercisePlan{
			Name:   ex.Name,
			Weight: weightsB[ex.Name],
		})
	}

	w.Header().Set("Content-Type", "text/html")
	weightUnit, err := h.weightUnitForUser(r, user.UserID)
	if err != nil {
		http.Error(w, "Failed to load unit preference", http.StatusInternalServerError)
		return
	}
	distanceUnit, err := h.distanceUnitForUser(r, user.UserID)
	if err != nil {
		distanceUnit = "mi"
	}
	templates.Render(w, "dashboard.html", map[string]any{
		"User":               user,
		"Plan":               plan,
		"PlanA":              planA,
		"PlanB":              planB,
		"StandaloneWorkouts": customWorkouts,
		"RecentSessions":     recentSessions,
		"CurrentPage":        currentPage,
		"PageLinks":          buildDashboardPageLinks(currentPage, totalSessions),
		"OpenWorkoutCount":   openWorkoutCount,
		"WeightUnit":         weightUnit,
		"DistanceUnit":       distanceUnit,
		"FinishSummary":      finishSummary,
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func parseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseTrackingSetUpdates(r *http.Request) ([]trackingSetInput, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		var body struct {
			Sets []trackingSetInput `json:"sets"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		return body.Sets, nil
	}

	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(r.FormValue("sets_json"))
	if raw == "" {
		return nil, nil
	}

	var updates []trackingSetInput
	if err := json.Unmarshal([]byte(raw), &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func valueAt(values []string, idx int) string {
	if idx < 0 || idx >= len(values) {
		return ""
	}
	return values[idx]
}

func parsePositiveIntWithDefault(s string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parseNonNegativeFloatWithDefault(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func parsePositiveFloatWithDefault(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func defaultExerciseName(exerciseType string) string {
	switch exerciseType {
	case workout.StandaloneTypeTreadmill:
		return "Treadmill"
	case workout.StandaloneTypeStaircase:
		return "Staircase"
	case workout.StandaloneTypeBike:
		return "Exercise Bike"
	case workout.StandaloneTypeElliptical:
		return "Elliptical"
	default:
		return "Custom Exercise"
	}
}
