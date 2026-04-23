package workout_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"stronglifts/internal/auth"
	"stronglifts/internal/db"
	"stronglifts/internal/handlers"

	"github.com/go-chi/chi/v5"
)

// --- test harness ---

type testApp struct {
	client  *http.Client
	server  *httptest.Server
	db      *db.DB
	cleanup func()
}

type transport struct{ server *httptest.Server }

type onboardingWeights struct {
	Squat    string
	Bench    string
	Row      string
	Press    string
	Deadlift string
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.server.URL, "http://")
	return http.DefaultTransport.RoundTrip(req)
}

func newApp(t *testing.T) *testApp {
	t.Helper()
	f, _ := os.CreateTemp("", "test_*.db")
	f.Close()

	testDB, err := db.New(f.Name())
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := testDB.CreateSchema(); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}

	sessionStore := auth.NewSessionStore(testDB.Conn())
	authH := handlers.NewAuthHandlers(testDB, sessionStore)
	workoutH := handlers.NewWorkoutHandlers(testDB)

	r := chi.NewRouter()
	r.Get("/register", authH.Register)
	r.Post("/register", authH.Register)
	r.Get("/login", authH.Login)
	r.Post("/login", authH.Login)
	r.Post("/logout", authH.Logout)
	r.Group(func(r chi.Router) {
		r.Use(auth.SessionMiddleware(sessionStore))
		r.Get("/onboarding", authH.Onboarding)
		r.Post("/onboarding", authH.Onboarding)
		r.Get("/", authH.Dashboard)
		r.Get("/workout/next", workoutH.NextWorkout)
		r.Get("/workout/{id}", workoutH.WorkoutPage)
		r.Get("/workout/standalone/new", workoutH.StandaloneEditor)
		r.Post("/workout/standalone", workoutH.CreateStandaloneWorkout)
		r.Post("/workout/standalone/delete-all", workoutH.DeleteAllStandaloneWorkouts)
		r.Get("/workout/standalone/{id}/edit", workoutH.EditStandaloneWorkout)
		r.Post("/workout/standalone/{id}", workoutH.UpdateStandaloneWorkout)
		r.Post("/workout/standalone/{id}/delete", workoutH.DeleteStandaloneWorkout)
		r.Post("/workout/start", workoutH.StartWorkout)
		r.Post("/workout/finish-open", workoutH.FinishOpenWorkouts)
		r.Post("/workout/{id}/save", workoutH.SaveTracking)
		r.Post("/workout/{id}/exercise/add", workoutH.AddExercise)
		r.Post("/workout/{id}/exercise/{group}/set/add", workoutH.AddSetToExercise)
		r.Post("/workout/{id}/exercise/reorder", workoutH.ReorderExercises)
		r.Delete("/workout/{id}/exercise/{group}", workoutH.DeleteExercise)
		r.Delete("/workout/{id}/set/{n}", workoutH.DeleteSet)
		r.Post("/workout/{id}/set/{n}/complete", workoutH.CompleteSet)
		r.Post("/workout/{id}/finish", workoutH.FinishWorkout)
		r.Delete("/workout/{id}", workoutH.DeleteWorkout)
		r.Post("/progress/{exercise}/deload", workoutH.DeloadExercise)
		r.Post("/progress/{exercise}/skip-increment", workoutH.ToggleSkipIncrement)
		r.Get("/backup/export", workoutH.ExportBackup)
		r.Post("/backup/import", workoutH.ImportBackup)
	})

	server := httptest.NewServer(r)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &transport{server: server},
	}

	return &testApp{
		client: client,
		server: server,
		db:     testDB,
		cleanup: func() {
			server.Close()
			testDB.Close()
			os.Remove(f.Name())
		},
	}
}

// registerAndLogin creates a user and returns the session cookie string
func (a *testApp) registerAndLogin(t *testing.T, username string) string {
	t.Helper()
	return a.registerAndLoginWithOnboarding(t, username, "lb_in", onboardingWeights{
		Squat:    "195",
		Bench:    "135",
		Row:      "95",
		Press:    "95",
		Deadlift: "225",
	})
}

func (a *testApp) registerAndLoginWithOnboarding(t *testing.T, username, unitPref string, w onboardingWeights) string {
	t.Helper()
	resp, _ := a.client.PostForm("http://app/register", url.Values{
		"username": {username},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = a.client.PostForm("http://app/login", url.Values{
		"username": {username},
		"password": {"password123"},
	})

	var sessionCookie string
	for _, c := range resp.Cookies() {
		if c.Name == auth.GetSessionCookieName() {
			sessionCookie = c.Name + "=" + c.Value
			break
		}
	}
	if sessionCookie == "" {
		resp.Body.Close()
		t.Fatal("no session cookie after login")
	}

	// Complete onboarding so workout flows are deterministic for tests.
	onboardingForm := url.Values{
		"unit_pref": {unitPref},
		"squat":     {w.Squat},
		"bench":     {w.Bench},
		"row":       {w.Row},
		"press":     {w.Press},
		"deadlift":  {w.Deadlift},
	}
	req, _ := http.NewRequest("POST", "http://app/onboarding", strings.NewReader(onboardingForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", sessionCookie)
	onboardResp, err := a.client.Do(req)
	if err != nil {
		resp.Body.Close()
		t.Fatalf("POST /onboarding: %v", err)
	}
	onboardResp.Body.Close()
	resp.Body.Close()
	return sessionCookie
}

// get performs an authenticated GET
func (a *testApp) get(t *testing.T, path, cookie string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://app"+path, nil)
	req.Header.Set("Cookie", cookie)
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// postJSON performs an authenticated POST with a JSON body
func (a *testApp) postJSON(t *testing.T, path, cookie string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "http://app"+path, bytes.NewReader(b))
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// postForm performs an authenticated POST with form-urlencoded body
func (a *testApp) postForm(t *testing.T, path, cookie string, form url.Values) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", "http://app"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// delete performs an authenticated DELETE
func (a *testApp) delete(t *testing.T, path, cookie string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("DELETE", "http://app"+path, nil)
	req.Header.Set("Cookie", cookie)
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

func (a *testApp) sessionSetCount(t *testing.T, sessionID int) int {
	t.Helper()
	var total int
	err := a.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ?`, sessionID).Scan(&total)
	if err != nil {
		t.Fatalf("count session sets: %v", err)
	}
	return total
}

// decodeJSON reads the response body as JSON into v
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("decode JSON (status %d): %v — body: %s", resp.StatusCode, err, body)
	}
}

// --- tests ---

func TestNextWorkoutIsAOnFirstCall(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "lifter")

	resp := app.get(t, "/workout/next", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	decodeJSON(t, resp, &result)

	if result["program"] != "A" {
		t.Errorf("expected program A on first call, got %v", result["program"])
	}
	exercises, ok := result["exercises"].([]any)
	if !ok || len(exercises) != 3 {
		t.Errorf("expected 3 exercises, got %v", result["exercises"])
	}
}

func TestStartWorkoutCreatesSession(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "lifter")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var result map[string]any
	decodeJSON(t, resp, &result)

	sessionID, ok := result["sessionId"].(float64)
	if !ok || sessionID == 0 {
		t.Errorf("expected numeric sessionId, got %v", result["sessionId"])
	}
}

func TestStartWorkoutProgramSelection(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "pickeruser")

	resp := app.postJSON(t, "/workout/start", cookie, map[string]any{
		"workoutType": "program",
		"programName": "B",
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	decodeJSON(t, resp, &result)
	sessionID := int64(result["sessionId"].(float64))

	var workoutName string
	err := app.db.Conn().QueryRow(`SELECT workout_name FROM workout_sessions WHERE id = ?`, sessionID).Scan(&workoutName)
	if err != nil {
		t.Fatalf("query workout_name: %v", err)
	}
	if workoutName != "B" {
		t.Fatalf("expected workout B, got %s", workoutName)
	}

	var squatSets, ohpSets, deadliftSets int
	if err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ? AND exercise_name = ?`, sessionID, "Squat").Scan(&squatSets); err != nil {
		t.Fatalf("count squat sets: %v", err)
	}
	if err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ? AND exercise_name = ?`, sessionID, "OHP").Scan(&ohpSets); err != nil {
		t.Fatalf("count OHP sets: %v", err)
	}
	if err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ? AND exercise_name = ?`, sessionID, "Deadlift").Scan(&deadliftSets); err != nil {
		t.Fatalf("count deadlift sets: %v", err)
	}

	if squatSets != 5 || ohpSets != 5 || deadliftSets != 1 {
		t.Fatalf("expected B-day set split 5/5/1, got squat=%d ohp=%d deadlift=%d", squatSets, ohpSets, deadliftSets)
	}
}

func TestCompleteSet(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "lifter")

	// Start a session
	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	// Complete set 1 of the session
	resp = app.postJSON(t,
		fmt.Sprintf("/workout/%d/set/1/complete", sessionID),
		cookie,
		map[string]any{"actualReps": 5, "weight": 20.0},
	)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestStandaloneEditorPage(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "standaloneuser")

	resp := app.get(t, "/workout/standalone/new", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Standalone Workout Editor") {
		t.Fatalf("expected editor page content, body: %s", body)
	}
}

func TestCreateStandaloneWorkoutSavesCardioDefaults(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "standalonecardio")

	form := url.Values{
		"title":          {"Conditioning Day"},
		"notes":          {"Conditioning day"},
		"exercise_type":  {"treadmill", "bike", "staircase"},
		"exercise_name":  {"Treadmill", "Exercise Bike", "Staircase"},
		"sets":           {"", "", ""},
		"target_reps":    {"", "", ""},
		"weight":         {"", "", ""},
		"time_minutes":   {"", "12", ""},
		"distance_miles": {"", "", ""},
	}
	req, _ := http.NewRequest("POST", "http://app/workout/standalone", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)

	resp, err := app.client.Do(req)
	if err != nil {
		t.Fatalf("POST /workout/standalone: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var workoutID int64
	err = app.db.Conn().QueryRow(`SELECT id FROM standalone_workouts ORDER BY id DESC LIMIT 1`).Scan(&workoutID)
	if err != nil {
		t.Fatalf("query standalone_workouts: %v", err)
	}

	rows, err := app.db.Conn().Query(`SELECT exercise_type, time_minutes, distance_miles FROM standalone_workout_items WHERE workout_id = ? ORDER BY position`, workoutID)
	if err != nil {
		t.Fatalf("query standalone_workout_items: %v", err)
	}
	defer rows.Close()

	type rowData struct {
		typ      string
		timeMins int
		distance float64
	}
	var got []rowData
	for rows.Next() {
		var r rowData
		if err := rows.Scan(&r.typ, &r.timeMins, &r.distance); err != nil {
			t.Fatalf("scan item: %v", err)
		}
		got = append(got, r)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 standalone items, got %d", len(got))
	}

	if got[0].typ != "treadmill" || got[0].timeMins != 10 || got[0].distance != 1 {
		t.Fatalf("unexpected treadmill defaults: %+v", got[0])
	}
	if got[1].typ != "bike" || got[1].timeMins != 12 || got[1].distance != 1 {
		t.Fatalf("unexpected bike defaults: %+v", got[1])
	}
	if got[2].typ != "staircase" || got[2].timeMins != 10 || got[2].distance != 0 {
		t.Fatalf("unexpected staircase defaults: %+v", got[2])
	}
}

func TestStartWorkoutFromStandaloneSelection(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "custompicker")

	form := url.Values{
		"title":          {"Custom Leg Day"},
		"notes":          {"Custom leg day"},
		"exercise_type":  {"strength"},
		"exercise_name":  {"Squat"},
		"sets":           {"3"},
		"target_reps":    {"8"},
		"weight":         {"185"},
		"time_minutes":   {""},
		"distance_miles": {""},
	}
	req, _ := http.NewRequest("POST", "http://app/workout/standalone", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	resp, err := app.client.Do(req)
	if err != nil {
		t.Fatalf("POST /workout/standalone: %v", err)
	}
	resp.Body.Close()

	var standaloneID int64
	err = app.db.Conn().QueryRow(`SELECT id FROM standalone_workouts ORDER BY id DESC LIMIT 1`).Scan(&standaloneID)
	if err != nil {
		t.Fatalf("query standalone id: %v", err)
	}

	resp = app.postJSON(t, "/workout/start", cookie, map[string]any{
		"workoutType":  "standalone",
		"standaloneId": standaloneID,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	decodeJSON(t, resp, &result)
	sessionID := int64(result["sessionId"].(float64))

	var setCount int
	err = app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ?`, sessionID).Scan(&setCount)
	if err != nil {
		t.Fatalf("count sets for custom session: %v", err)
	}
	if setCount != 3 {
		t.Fatalf("expected 3 sets from standalone workout, got %d", setCount)
	}
}

func TestStartWorkoutFromStandaloneStrengthScheme(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "schemepicker")

	schemeJSON := `[{"position":1,"targetReps":8,"weight":185},{"position":2,"targetReps":10,"weight":165},{"position":3,"targetReps":12,"weight":145}]`
	form := url.Values{
		"title":           {"Reverse Pyramid Bench"},
		"notes":           {"Heavy to light"},
		"exercise_type":   {"strength"},
		"exercise_name":   {"Bench Press"},
		"sets":            {"3"},
		"target_reps":     {"8"},
		"weight":          {"185"},
		"set_scheme_json": {schemeJSON},
		"time_minutes":    {""},
		"distance_miles":  {""},
	}
	req, _ := http.NewRequest("POST", "http://app/workout/standalone", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	resp, err := app.client.Do(req)
	if err != nil {
		t.Fatalf("POST /workout/standalone: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var workoutID int64
	err = app.db.Conn().QueryRow(`SELECT id FROM standalone_workouts ORDER BY id DESC LIMIT 1`).Scan(&workoutID)
	if err != nil {
		t.Fatalf("query standalone workout id: %v", err)
	}

	rows, err := app.db.Conn().Query(`
		SELECT s.position, s.target_reps, s.weight
		FROM standalone_workout_item_sets s
		JOIN standalone_workout_items i ON i.id = s.workout_item_id
		WHERE i.workout_id = ?
		ORDER BY s.position`, workoutID)
	if err != nil {
		t.Fatalf("query standalone_workout_item_sets: %v", err)
	}
	defer rows.Close()

	type schemeRow struct {
		position int
		reps     int
		weight   float64
	}
	var gotScheme []schemeRow
	for rows.Next() {
		var row schemeRow
		if err := rows.Scan(&row.position, &row.reps, &row.weight); err != nil {
			t.Fatalf("scan standalone scheme row: %v", err)
		}
		gotScheme = append(gotScheme, row)
	}
	if len(gotScheme) != 3 {
		t.Fatalf("expected 3 scheme rows, got %d", len(gotScheme))
	}
	if gotScheme[0].reps != 8 || gotScheme[0].weight != 185 || gotScheme[2].reps != 12 || gotScheme[2].weight != 145 {
		t.Fatalf("unexpected saved scheme rows: %+v", gotScheme)
	}

	resp = app.postJSON(t, "/workout/start", cookie, map[string]any{
		"workoutType":  "standalone",
		"standaloneId": workoutID,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	decodeJSON(t, resp, &result)
	sessionID := int64(result["sessionId"].(float64))

	setRows, err := app.db.Conn().Query(`SELECT set_number, target_reps, weight FROM exercise_sets WHERE session_id = ? ORDER BY set_number`, sessionID)
	if err != nil {
		t.Fatalf("query exercise_sets for session: %v", err)
	}
	defer setRows.Close()
	var targetReps []int
	var weights []float64
	for setRows.Next() {
		var setNum int
		var reps int
		var weight float64
		if err := setRows.Scan(&setNum, &reps, &weight); err != nil {
			t.Fatalf("scan exercise_set: %v", err)
		}
		targetReps = append(targetReps, reps)
		weights = append(weights, weight)
	}
	if len(targetReps) != 3 {
		t.Fatalf("expected 3 session sets from strength scheme, got %d", len(targetReps))
	}
	if targetReps[0] != 8 || targetReps[1] != 10 || targetReps[2] != 12 {
		t.Fatalf("unexpected target reps from scheme: %v", targetReps)
	}
	if weights[0] != 185 || weights[1] != 165 || weights[2] != 145 {
		t.Fatalf("unexpected weights from scheme: %v", weights)
	}
}

func TestEditStandaloneWorkoutPageAndUpdate(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "editstandalone")

	resp := app.postForm(t, "/workout/standalone", cookie, url.Values{
		"title":           {"Original Workout"},
		"notes":           {"Original notes"},
		"exercise_type":   {"strength"},
		"exercise_name":   {"Squat"},
		"sets":            {"2"},
		"target_reps":     {"5"},
		"weight":          {"225"},
		"set_scheme_json": {`[{"position":1,"targetReps":5,"weight":225},{"position":2,"targetReps":8,"weight":205}]`},
		"time_minutes":    {""},
		"distance_miles":  {""},
	})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 creating standalone workout, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var workoutID int64
	err := app.db.Conn().QueryRow(`SELECT id FROM standalone_workouts ORDER BY id DESC LIMIT 1`).Scan(&workoutID)
	if err != nil {
		t.Fatalf("query standalone workout id: %v", err)
	}

	resp = app.get(t, fmt.Sprintf("/workout/standalone/%d/edit", workoutID), cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on edit page, got %d: %s", resp.StatusCode, body)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Edit Standalone Workout") || !strings.Contains(bodyStr, "Original Workout") {
		t.Fatalf("expected edit page to be prefilled, body: %s", body)
	}

	resp = app.postForm(t, fmt.Sprintf("/workout/standalone/%d", workoutID), cookie, url.Values{
		"title":           {"Updated Workout"},
		"notes":           {"Updated notes"},
		"exercise_type":   {"strength", "bike"},
		"exercise_name":   {"Bench Press", "Exercise Bike"},
		"sets":            {"3", ""},
		"target_reps":     {"6", ""},
		"weight":          {"185", ""},
		"set_scheme_json": {`[{"position":1,"targetReps":6,"weight":185},{"position":2,"targetReps":8,"weight":165},{"position":3,"targetReps":10,"weight":145}]`, ""},
		"time_minutes":    {"", "15"},
		"distance_miles":  {"", "2"},
	})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 updating standalone workout, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var title, notes string
	err = app.db.Conn().QueryRow(`SELECT title, notes FROM standalone_workouts WHERE id = ?`, workoutID).Scan(&title, &notes)
	if err != nil {
		t.Fatalf("query updated standalone workout: %v", err)
	}
	if title != "Updated Workout" || notes != "Updated notes" {
		t.Fatalf("expected updated title/notes, got %q / %q", title, notes)
	}

	var itemCount int
	err = app.db.Conn().QueryRow(`SELECT COUNT(1) FROM standalone_workout_items WHERE workout_id = ?`, workoutID).Scan(&itemCount)
	if err != nil {
		t.Fatalf("count updated standalone items: %v", err)
	}
	if itemCount != 2 {
		t.Fatalf("expected 2 updated standalone items, got %d", itemCount)
	}

	var schemeCount int
	err = app.db.Conn().QueryRow(`
		SELECT COUNT(1)
		FROM standalone_workout_item_sets s
		JOIN standalone_workout_items i ON i.id = s.workout_item_id
		WHERE i.workout_id = ?`, workoutID).Scan(&schemeCount)
	if err != nil {
		t.Fatalf("count updated standalone scheme rows: %v", err)
	}
	if schemeCount != 3 {
		t.Fatalf("expected 3 updated scheme rows, got %d", schemeCount)
	}
}

func TestDeleteStandaloneWorkout(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "deletestandalone")

	for i := 0; i < 2; i++ {
		resp := app.postForm(t, "/workout/standalone", cookie, url.Values{
			"title":           {fmt.Sprintf("Standalone %d", i+1)},
			"notes":           {""},
			"exercise_type":   {"strength"},
			"exercise_name":   {"Squat"},
			"sets":            {"1"},
			"target_reps":     {"5"},
			"weight":          {"225"},
			"set_scheme_json": {`[{"position":1,"targetReps":5,"weight":225}]`},
			"time_minutes":    {""},
			"distance_miles":  {""},
		})
		if resp.StatusCode != http.StatusFound {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 302 creating standalone workout %d, got %d: %s", i+1, resp.StatusCode, body)
		}
		resp.Body.Close()
	}

	var workoutID int64
	err := app.db.Conn().QueryRow(`SELECT id FROM standalone_workouts ORDER BY id ASC LIMIT 1`).Scan(&workoutID)
	if err != nil {
		t.Fatalf("query standalone workout id: %v", err)
	}

	resp := app.postForm(t, fmt.Sprintf("/workout/standalone/%d/delete", workoutID), cookie, url.Values{})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 deleting standalone workout, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var count int
	err = app.db.Conn().QueryRow(`SELECT COUNT(1) FROM standalone_workouts`).Scan(&count)
	if err != nil {
		t.Fatalf("count standalone workouts after delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 standalone workout remaining, got %d", count)
	}

	resp = app.postForm(t, "/workout/standalone/delete-all", cookie, url.Values{})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 deleting all standalone workouts, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	err = app.db.Conn().QueryRow(`SELECT COUNT(1) FROM standalone_workouts`).Scan(&count)
	if err != nil {
		t.Fatalf("count standalone workouts after bulk delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 standalone workouts after bulk delete, got %d", count)
	}
}

func TestFinishWorkoutWithAllSetsIncrementsWeight(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "lifter")

	// Start
	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	totalSets := app.sessionSetCount(t, sessionID)
	for setNum := 1; setNum <= totalSets; setNum++ {
		resp = app.postJSON(t,
			fmt.Sprintf("/workout/%d/set/%d/complete", sessionID, setNum),
			cookie,
			map[string]any{"actualReps": 5, "weight": 20.0},
		)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("set %d: expected 200, got %d", setNum, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Finish
	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/finish", sessionID), cookie, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	decodeJSON(t, resp, &result)

	updates, ok := result["progressUpdates"].([]any)
	if !ok || len(updates) != 3 {
		t.Fatalf("expected 3 progress updates, got %v", result["progressUpdates"])
	}

	// Imperial progression should increase A-day lifts by 5.0.
	checked := 0
	for _, u := range updates {
		update := u.(map[string]any)
		if update["Action"] != "increased" {
			t.Fatalf("expected increased action for completed set block, got %v", update["Action"])
		}
		oldWeight := update["OldWeight"].(float64)
		newWeight := update["NewWeight"].(float64)
		delta := newWeight - oldWeight
		if delta != 5.0 {
			t.Fatalf("expected imperial increment of 5.0, got %.2f for %v", delta, update["ExerciseName"])
		}
		checked++
	}
	if checked != 3 {
		t.Fatalf("expected to validate 3 updates, validated %d", checked)
	}
}

func TestFinishWorkoutWithAllSetsIncrementsWeightMetric(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLoginWithOnboarding(t, "metriclifter", "kg_cm", onboardingWeights{
		Squat:    "100",
		Bench:    "60",
		Row:      "60",
		Press:    "40",
		Deadlift: "120",
	})

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	totalSets := app.sessionSetCount(t, sessionID)
	for setNum := 1; setNum <= totalSets; setNum++ {
		resp = app.postJSON(t,
			fmt.Sprintf("/workout/%d/set/%d/complete", sessionID, setNum),
			cookie,
			map[string]any{"actualReps": 5, "weight": 20.0},
		)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("set %d: expected 200, got %d", setNum, resp.StatusCode)
		}
		resp.Body.Close()
	}

	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/finish", sessionID), cookie, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	decodeJSON(t, resp, &result)

	updates, ok := result["progressUpdates"].([]any)
	if !ok || len(updates) != 3 {
		t.Fatalf("expected 3 progress updates, got %v", result["progressUpdates"])
	}

	for _, u := range updates {
		update := u.(map[string]any)
		if update["Action"] != "increased" {
			t.Fatalf("expected increased action for completed set block, got %v", update["Action"])
		}
		oldWeight := update["OldWeight"].(float64)
		newWeight := update["NewWeight"].(float64)
		delta := newWeight - oldWeight
		if delta != 2.5 {
			t.Fatalf("expected metric increment of 2.5, got %.2f for %v", delta, update["ExerciseName"])
		}
	}
}

func TestFinishWorkoutBDeadliftIncrementImperial(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "deadliftincrementuser")

	resp := app.postJSON(t, "/workout/start", cookie, map[string]any{
		"workoutType": "program",
		"programName": "B",
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	totalSets := app.sessionSetCount(t, sessionID)
	for setNum := 1; setNum <= totalSets; setNum++ {
		resp = app.postJSON(t,
			fmt.Sprintf("/workout/%d/set/%d/complete", sessionID, setNum),
			cookie,
			map[string]any{"actualReps": 5, "weight": 20.0},
		)
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("set %d: expected 200, got %d: %s", setNum, resp.StatusCode, body)
		}
		resp.Body.Close()
	}

	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/finish", sessionID), cookie, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	decodeJSON(t, resp, &result)

	updates, ok := result["progressUpdates"].([]any)
	if !ok {
		t.Fatalf("expected progress updates in response, got %v", result["progressUpdates"])
	}

	for _, u := range updates {
		update := u.(map[string]any)
		if update["ExerciseName"] != "Deadlift" {
			continue
		}
		delta := update["NewWeight"].(float64) - update["OldWeight"].(float64)
		if delta != 5.0 {
			t.Fatalf("expected deadlift increment of 5.0 lb, got %.2f", delta)
		}
		return
	}

	t.Fatalf("did not find deadlift progress update in %v", updates)
}

func TestFinishWorkoutWithFailedSetsIncrementsFailStreak(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "lifter")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	totalSets := app.sessionSetCount(t, sessionID)
	for setNum := 1; setNum <= totalSets; setNum++ {
		resp = app.postJSON(t,
			fmt.Sprintf("/workout/%d/set/%d/complete", sessionID, setNum),
			cookie,
			map[string]any{"actualReps": 3, "weight": 20.0},
		)
		resp.Body.Close()
	}

	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/finish", sessionID), cookie, nil)
	var result map[string]any
	decodeJSON(t, resp, &result)

	updates := result["progressUpdates"].([]any)
	for _, u := range updates {
		update := u.(map[string]any)
		if update["Action"] != "unchanged" {
			t.Errorf("expected all unchanged on first failure, got action=%v for %v",
				update["Action"], update["ExerciseName"])
		}
	}
}

func TestDeloadAfterThreeFailures(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "lifter")

	// Run 3 workouts alternating A/B/A, failing all sets each time
	// After 3 failures on the same exercise, it should deload
	var lastSessionID int

	for w := 0; w < 3; w++ {
		resp := app.postJSON(t, "/workout/start", cookie, nil)
		var session map[string]any
		decodeJSON(t, resp, &session)
		lastSessionID = int(session["sessionId"].(float64))

		totalSets := app.sessionSetCount(t, lastSessionID)
		for setNum := 1; setNum <= totalSets; setNum++ {
			resp = app.postJSON(t,
				fmt.Sprintf("/workout/%d/set/%d/complete", lastSessionID, setNum),
				cookie,
				map[string]any{"actualReps": 3, "weight": 20.0},
			)
			resp.Body.Close()
		}

		resp = app.postJSON(t, fmt.Sprintf("/workout/%d/finish", lastSessionID), cookie, nil)
		resp.Body.Close()
	}

	// The 3rd finish should have triggered a deload for Squat (present in all workouts)
	// Re-run next workout to confirm deloaded weight is used
	resp := app.get(t, "/workout/next", cookie)
	var next map[string]any
	decodeJSON(t, resp, &next)

	exercises := next["exercises"].([]any)
	for _, e := range exercises {
		ex := e.(map[string]any)
		if ex["name"] == "Squat" {
			weight := ex["weight"].(float64)
			// Started at 195.0, deloaded below start after 3 failures.
			if weight >= 195.0 {
				t.Errorf("expected Squat deload from 195, got %v", weight)
			}
		}
	}
}

func TestNextWorkoutAlternatesAB(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "lifter")

	// First workout should be A
	resp := app.get(t, "/workout/next", cookie)
	var next map[string]any
	decodeJSON(t, resp, &next)
	if next["program"] != "A" {
		t.Fatalf("expected A, got %v", next["program"])
	}

	// Start and finish workout A
	resp = app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/finish", sessionID), cookie, nil)
	resp.Body.Close()

	// Next should be B
	resp = app.get(t, "/workout/next", cookie)
	var next2 map[string]any
	decodeJSON(t, resp, &next2)
	if next2["program"] != "B" {
		t.Errorf("expected B after finishing A, got %v", next2["program"])
	}
}

func TestDeleteWorkout(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "lifter")

	// Start a session
	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	// Delete it
	resp = app.delete(t, fmt.Sprintf("/workout/%d", sessionID), cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Completing a set on the deleted session should 404
	resp = app.postJSON(t,
		fmt.Sprintf("/workout/%d/set/1/complete", sessionID),
		cookie,
		map[string]any{"actualReps": 5, "weight": 20.0},
	)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCannotAccessOtherUsersWorkout(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()

	cookieA := app.registerAndLogin(t, "alice")
	cookieB := app.registerAndLogin(t, "bob")

	// Alice starts a session
	resp := app.postJSON(t, "/workout/start", cookieA, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	// Bob tries to complete a set in Alice's session
	resp = app.postJSON(t,
		fmt.Sprintf("/workout/%d/set/1/complete", sessionID),
		cookieB,
		map[string]any{"actualReps": 5, "weight": 20.0},
	)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 when accessing other user's session, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAddDuplicateExerciseCreatesNewExerciseBlock(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "dupblocks")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	resp = app.postForm(t, fmt.Sprintf("/workout/%d/exercise/add", sessionID), cookie, url.Values{
		"exercise_type": {"strength"},
		"exercise_name": {"Squat"},
		"sets":          {"2"},
		"target_reps":   {"5"},
		"weight":        {"205"},
	})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 on add exercise, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = app.get(t, fmt.Sprintf("/workout/%d", sessionID), cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on workout page, got %d: %s", resp.StatusCode, body)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if got := strings.Count(string(body), `data-exercise="Squat"`); got != 2 {
		t.Fatalf("expected 2 separate squat exercise blocks, got %d", got)
	}
}

func TestAddDuplicateExerciseWhenSameAsLastCreatesSeparateBlock(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "dupblockslast")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/exercise/reorder", sessionID), cookie, map[string]any{
		"from": 1,
		"to":   3,
	})
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on reorder, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = app.postForm(t, fmt.Sprintf("/workout/%d/exercise/add", sessionID), cookie, url.Values{
		"exercise_type": {"strength"},
		"exercise_name": {"Squat"},
		"sets":          {"2"},
		"target_reps":   {"5"},
		"weight":        {"205"},
	})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 on add exercise, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = app.get(t, fmt.Sprintf("/workout/%d", sessionID), cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on workout page, got %d: %s", resp.StatusCode, body)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if got := strings.Count(string(body), `data-exercise="Squat"`); got != 2 {
		t.Fatalf("expected 2 separate adjacent squat exercise blocks, got %d", got)
	}
}

func TestAddSetToExerciseGroupAppendsSet(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "addsetuser")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	resp = app.postForm(t, fmt.Sprintf("/workout/%d/exercise/1/set/add", sessionID), cookie, url.Values{})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 on add set, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var totalSets int
	if err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ?`, sessionID).Scan(&totalSets); err != nil {
		t.Fatalf("count sets after add set: %v", err)
	}
	if totalSets != 16 {
		t.Fatalf("expected 16 total sets after appending one set, got %d", totalSets)
	}

	var squatSets int
	if err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ? AND exercise_name = ?`, sessionID, "Squat").Scan(&squatSets); err != nil {
		t.Fatalf("count squat sets after add set: %v", err)
	}
	if squatSets != 6 {
		t.Fatalf("expected squat set count to become 6 after append, got %d", squatSets)
	}
}

func TestReorderExerciseBlocksUpdatesSetOrder(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "reorderuser")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/exercise/reorder", sessionID), cookie, map[string]any{
		"from": 1,
		"to":   3,
	})
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on reorder, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var firstExercise string
	err := app.db.Conn().QueryRow(`SELECT exercise_name FROM exercise_sets WHERE session_id = ? AND set_number = 1`, sessionID).Scan(&firstExercise)
	if err != nil {
		t.Fatalf("query first exercise after reorder: %v", err)
	}
	if firstExercise != "Bench Press" {
		t.Fatalf("expected Bench Press first after reorder, got %s", firstExercise)
	}
}

func TestDeleteExerciseBlockRemovesOnlySelectedBlock(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "deleteblockuser")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	resp = app.postForm(t, fmt.Sprintf("/workout/%d/exercise/add", sessionID), cookie, url.Values{
		"exercise_type": {"strength"},
		"exercise_name": {"Squat"},
		"sets":          {"2"},
		"target_reps":   {"5"},
		"weight":        {"205"},
	})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 on add exercise, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = app.delete(t, fmt.Sprintf("/workout/%d/exercise/4", sessionID), cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on delete exercise block, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var totalSets int
	err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ?`, sessionID).Scan(&totalSets)
	if err != nil {
		t.Fatalf("count sets after delete: %v", err)
	}
	if totalSets != 15 {
		t.Fatalf("expected total sets to return to 15 after deleting added block, got %d", totalSets)
	}
}

func TestFinishAllOpenWorkouts(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "finishalluser")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 starting first workout, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = app.postJSON(t, "/workout/start", cookie, map[string]any{
		"workoutType": "program",
		"programName": "B",
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 starting second workout, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = app.postForm(t, "/workout/finish-open", cookie, url.Values{})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 from finish-open, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var openCount int
	err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM workout_sessions WHERE finished_at IS NULL`).Scan(&openCount)
	if err != nil {
		t.Fatalf("count open sessions after finish-open: %v", err)
	}
	if openCount != 0 {
		t.Fatalf("expected no open sessions after finish-open, got %d", openCount)
	}
}

func TestDashboardRecentWorkoutsPaginatedToTenByDefault(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "paginateuser")

	for i := 0; i < 12; i++ {
		resp := app.postJSON(t, "/workout/start", cookie, nil)
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 201 creating session %d, got %d: %s", i+1, resp.StatusCode, body)
		}
		resp.Body.Close()
	}

	resp := app.get(t, "/", cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for dashboard, got %d: %s", resp.StatusCode, body)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	bodyStr := string(body)

	if got := strings.Count(bodyStr, `>Open</a>`); got != 10 {
		t.Fatalf("expected 10 recent workout open links on first page, got %d", got)
	}
	if !strings.Contains(bodyStr, `href="/?page=2">2</a>`) {
		t.Fatalf("expected dashboard to show numbered pagination link for page 2")
	}

	resp = app.get(t, "/?page=2", cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for dashboard page 2, got %d: %s", resp.StatusCode, body)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	bodyStr = string(body)

	if got := strings.Count(bodyStr, `>Open</a>`); got != 2 {
		t.Fatalf("expected 2 recent workout open links on second page, got %d", got)
	}
	if !strings.Contains(bodyStr, `href="/?page=1">1</a>`) {
		t.Fatalf("expected dashboard page 2 to show numbered pagination link back to page 1")
	}
	if !strings.Contains(bodyStr, `>2</span>`) {
		t.Fatalf("expected dashboard page 2 to highlight the current page")
	}
}

func TestDeleteSetRemovesOnlyRequestedSet(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "deletesetuser")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	resp = app.delete(t, fmt.Sprintf("/workout/%d/set/3", sessionID), cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on delete set, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var totalSets int
	err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ?`, sessionID).Scan(&totalSets)
	if err != nil {
		t.Fatalf("count sets after delete set: %v", err)
	}
	if totalSets != 14 {
		t.Fatalf("expected 14 sets after deleting one set, got %d", totalSets)
	}

	var deletedCount int
	err = app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ? AND set_number = 3`, sessionID).Scan(&deletedCount)
	if err != nil {
		t.Fatalf("count deleted set number: %v", err)
	}
	if deletedCount != 0 {
		t.Fatalf("expected set_number 3 to be deleted")
	}
}

func TestSkipNextIncrementPreventsOneIncrease(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "skipincrementuser")

	resp := app.postForm(t, "/progress/Squat/skip-increment", cookie, url.Values{"skip": {"1"}})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 when enabling skip increment, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	totalSets := app.sessionSetCount(t, sessionID)
	for setNum := 1; setNum <= totalSets; setNum++ {
		resp = app.postJSON(t,
			fmt.Sprintf("/workout/%d/set/%d/complete", sessionID, setNum),
			cookie,
			map[string]any{"actualReps": 5, "weight": 20.0},
		)
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("complete set %d: %d %s", setNum, resp.StatusCode, body)
		}
		resp.Body.Close()
	}

	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/finish", sessionID), cookie, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 finishing workout, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var squatWeight float64
	err := app.db.Conn().QueryRow(`SELECT current_weight FROM lift_progress WHERE exercise_name = 'Squat'`).Scan(&squatWeight)
	if err != nil {
		t.Fatalf("query squat progress: %v", err)
	}
	if squatWeight != 195 {
		t.Fatalf("expected squat to stay at 195 after skipped increment, got %.1f", squatWeight)
	}
}

func TestManualDeloadLowersWeight(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "manualdeloaduser")

	resp := app.postForm(t, "/progress/Squat/deload", cookie, url.Values{})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 when manual deload, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var squatWeight float64
	err := app.db.Conn().QueryRow(`SELECT current_weight FROM lift_progress WHERE exercise_name = 'Squat'`).Scan(&squatWeight)
	if err != nil {
		t.Fatalf("query squat after deload: %v", err)
	}
	if squatWeight >= 195 {
		t.Fatalf("expected squat to be deloaded below 195, got %.1f", squatWeight)
	}
}

func TestBackupExportAndImportRoundTrip(t *testing.T) {
	app := newApp(t)
	defer app.cleanup()
	cookie := app.registerAndLogin(t, "backupuser")

	resp := app.postJSON(t, "/workout/start", cookie, nil)
	var session map[string]any
	decodeJSON(t, resp, &session)
	sessionID := int(session["sessionId"].(float64))

	resp = app.postJSON(t, fmt.Sprintf("/workout/%d/set/1/complete", sessionID), cookie, map[string]any{"actualReps": 5, "weight": 20.0})
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 complete set before backup, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = app.get(t, "/backup/export", cookie)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 backup export, got %d: %s", resp.StatusCode, body)
	}
	backupPayload, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(backupPayload), `"sessions"`) {
		t.Fatalf("expected backup payload to contain sessions key")
	}

	if _, err := app.db.Conn().Exec(`DELETE FROM exercise_sets`); err != nil {
		t.Fatalf("delete exercise_sets before import: %v", err)
	}
	if _, err := app.db.Conn().Exec(`DELETE FROM workout_sessions`); err != nil {
		t.Fatalf("delete workout_sessions before import: %v", err)
	}

	resp = app.postForm(t, "/backup/import", cookie, url.Values{"backup_json": {string(backupPayload)}})
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302 backup import, got %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var restoredSets int
	err := app.db.Conn().QueryRow(`SELECT COUNT(1) FROM exercise_sets`).Scan(&restoredSets)
	if err != nil {
		t.Fatalf("count restored sets after backup import: %v", err)
	}
	if restoredSets == 0 {
		t.Fatalf("expected exercise sets to be restored after backup import")
	}
}
