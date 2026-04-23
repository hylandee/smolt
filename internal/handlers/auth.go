package handlers

import (
	"net/http"
	"strconv"
	"stronglifts/internal/auth"
	"stronglifts/internal/db"
	"stronglifts/internal/templates"
	"stronglifts/internal/workout"
)

const dashboardRecentSessionsPageSize = 10

type dashboardPageLink struct {
	Number  int
	Current bool
}

func parseDashboardPage(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func clampDashboardPage(currentPage, totalSessions int) int {
	totalPages := 1
	if totalSessions > 0 {
		totalPages = (totalSessions + dashboardRecentSessionsPageSize - 1) / dashboardRecentSessionsPageSize
	}
	if currentPage < 1 {
		return 1
	}
	if currentPage > totalPages {
		return totalPages
	}
	return currentPage
}

func buildDashboardPageLinks(currentPage, totalSessions int) []dashboardPageLink {
	totalPages := (totalSessions + dashboardRecentSessionsPageSize - 1) / dashboardRecentSessionsPageSize
	if totalPages <= 1 {
		return nil
	}

	links := make([]dashboardPageLink, 0, totalPages)
	for page := 1; page <= totalPages; page++ {
		links = append(links, dashboardPageLink{
			Number:  page,
			Current: page == currentPage,
		})
	}
	return links
}

type AuthHandlers struct {
	db           *db.DB
	authService  *auth.AuthService
	sessionStore *auth.SessionStore
	progression  *workout.ProgressionService
}

func NewAuthHandlers(database *db.DB, sessionStore *auth.SessionStore) *AuthHandlers {
	return &AuthHandlers{
		db:           database,
		authService:  auth.NewAuthService(database.Conn()),
		sessionStore: sessionStore,
		progression:  workout.NewProgressionService(database.Conn()),
	}
}

func writeSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.GetSessionCookieName(),
		Value:    sessionID,
		Path:     "/",
		MaxAge:   auth.SessionMaxAgeSeconds(),
		HttpOnly: true,
	})
}

func parseWeight(formVal string, fallback float64) (float64, error) {
	if formVal == "" {
		return fallback, nil
	}
	v, err := strconv.ParseFloat(formVal, 64)
	if err != nil {
		return 0, err
	}
	if v <= 0 {
		return 0, strconv.ErrSyntax
	}
	return v, nil
}

func (h *AuthHandlers) renderProfile(w http.ResponseWriter, r *http.Request, user *auth.UserSession, errorMsg, savedMsg string) {
	unitPref, err := h.authService.GetUnitPref(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load profile", http.StatusInternalServerError)
		return
	}
	distanceUnitPref, err := h.authService.GetDistanceUnitPref(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	templates.Render(w, "profile.html", map[string]any{
		"User":             user,
		"UnitPref":         unitPref,
		"DistanceUnitPref": distanceUnitPref,
		"Saved":            savedMsg,
		"Error":            errorMsg,
	})
}

// Register handles GET/POST /register
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		templates.Render(w, "register.html", map[string]any{"User": nil})
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	renderErr := func(msg string) {
		w.Header().Set("Content-Type", "text/html")
		templates.Render(w, "register.html", map[string]any{
			"User":     nil,
			"Error":    msg,
			"Username": username,
		})
	}

	if len(username) < 3 {
		renderErr("Username must be at least 3 characters")
		return
	}
	if len(password) < 3 {
		renderErr("Password must be at least 3 characters")
		return
	}

	confirm := r.FormValue("confirm")
	if confirm != "" && confirm != password {
		renderErr("Passwords do not match")
		return
	}

	_, err := h.authService.RegisterUser(r.Context(), username, password)
	if err == auth.ErrUserExists {
		renderErr("Username already taken")
		return
	}
	if err != nil {
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}

// Login handles GET/POST /login
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		templates.Render(w, "login.html", map[string]any{"User": nil})
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	renderErr := func(msg string) {
		w.Header().Set("Content-Type", "text/html")
		templates.Render(w, "login.html", map[string]any{
			"User":     nil,
			"Error":    msg,
			"Username": username,
		})
	}

	user, err := h.authService.GetUser(r.Context(), username)
	if err != nil || h.authService.VerifyPassword(password, user.PasswordHash) != nil {
		renderErr("Invalid username or password")
		return
	}

	sessionID, err := h.sessionStore.Create(auth.UserSession{
		UserID:   user.ID,
		Username: user.Username,
	})
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	writeSessionCookie(w, sessionID)

	http.Redirect(w, r, "/workouts", http.StatusFound)
}

// Logout handles POST /logout
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(auth.GetSessionCookieName())
	if cookie != nil {
		_ = h.sessionStore.Delete(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.GetSessionCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/login", http.StatusFound)
}

// DeleteAccount handles POST /account/delete and soft-deletes the current user.
func (h *AuthHandlers) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := h.authService.SoftDeleteUser(r.Context(), user.UserID); err != nil {
		http.Error(w, "Failed to delete account", http.StatusInternalServerError)
		return
	}

	// Invalidate all active sessions for this user.
	if err := h.sessionStore.DeleteByUserID(user.UserID); err != nil {
		http.Error(w, "Failed to delete account sessions", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.GetSessionCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/login", http.StatusFound)
}

// Profile handles GET/POST /profile for account settings.
func (h *AuthHandlers) Profile(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		switch r.FormValue("action") {
		case "password":
			err := h.authService.ChangePassword(
				r.Context(),
				user.UserID,
				r.FormValue("current_password"),
				r.FormValue("new_password"),
				r.FormValue("confirm_password"),
			)
			if err != nil {
				message := "Failed to update password"
				switch err {
				case auth.ErrInvalidCredentials:
					message = "Current password is incorrect"
				case auth.ErrInvalidPassword:
					message = "New password must be at least 3 characters"
				case auth.ErrPasswordMismatch:
					message = "New passwords do not match"
				case auth.ErrPasswordUnchanged:
					message = "New password must be different from current password"
				}
				h.renderProfile(w, r, user, message, "")
				return
			}
			// Drop other sessions so only the current request remains trusted.
			if err := h.sessionStore.DeleteByUserID(user.UserID); err != nil {
				http.Error(w, "Failed to rotate sessions", http.StatusInternalServerError)
				return
			}
			sessionID, err := h.sessionStore.Create(*user)
			if err != nil {
				http.Error(w, "Failed to create session", http.StatusInternalServerError)
				return
			}
			writeSessionCookie(w, sessionID)
			http.Redirect(w, r, "/profile?password_saved=1", http.StatusFound)
			return
		default:
			unitPref := r.FormValue("unit_pref")
			if err := h.authService.UpdateUnitPref(r.Context(), user.UserID, unitPref); err != nil {
				h.renderProfile(w, r, user, "Invalid unit preference", "")
				return
			}
			distanceUnitPref := r.FormValue("distance_unit_pref")
			if distanceUnitPref == "" {
				distanceUnitPref, _ = h.authService.GetDistanceUnitPref(r.Context(), user.UserID)
			}
			if err := h.authService.UpdateDistanceUnitPref(r.Context(), user.UserID, distanceUnitPref); err != nil {
				h.renderProfile(w, r, user, "Invalid distance unit preference", "")
				return
			}

			http.Redirect(w, r, "/profile?settings_saved=1", http.StatusFound)
			return
		}
	}

	savedMsg := ""
	switch {
	case r.URL.Query().Get("settings_saved") == "1":
		savedMsg = "Settings saved."
	case r.URL.Query().Get("password_saved") == "1":
		savedMsg = "Password updated."
	}
	h.renderProfile(w, r, user, "", savedMsg)
}

// Onboarding handles GET/POST /onboarding for first-workout setup.
func (h *AuthHandlers) Onboarding(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	initialized, err := h.progression.IsInitialized(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load onboarding state", http.StatusInternalServerError)
		return
	}
	if initialized && r.Method == http.MethodGet {
		http.Redirect(w, r, "/workouts", http.StatusFound)
		return
	}

	render := func(errMsg string) {
		unitPref, _ := h.authService.GetUnitPref(r.Context(), user.UserID)
		distanceUnitPref, _ := h.authService.GetDistanceUnitPref(r.Context(), user.UserID)
		w.Header().Set("Content-Type", "text/html")
		templates.Render(w, "onboarding.html", map[string]any{
			"User":             user,
			"Error":            errMsg,
			"UnitPref":         unitPref,
			"DistanceUnitPref": distanceUnitPref,
			"Squat":            r.FormValue("squat"),
			"Bench":            r.FormValue("bench"),
			"Row":              r.FormValue("row"),
			"Press":            r.FormValue("press"),
			"Deadlift":         r.FormValue("deadlift"),
			"DefSquat":         workout.Squat.DefaultStartWeight,
			"DefBench":         workout.BenchPress.DefaultStartWeight,
			"DefRow":           workout.BarbellRow.DefaultStartWeight,
			"DefPress":         workout.OHP.DefaultStartWeight,
			"DefDead":          workout.Deadlift.DefaultStartWeight,
		})
	}

	if r.Method == http.MethodGet {
		render("")
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	if err := h.authService.UpdateUnitPref(r.Context(), user.UserID, r.FormValue("unit_pref")); err != nil {
		render("Please choose valid units")
		return
	}
	distanceUnitPref := r.FormValue("distance_unit_pref")
	if distanceUnitPref == "" {
		if pref, prefErr := h.authService.GetDistanceUnitPref(r.Context(), user.UserID); prefErr == nil {
			distanceUnitPref = pref
		} else {
			distanceUnitPref = auth.DistanceUnitMiles
		}
	}
	if err := h.authService.UpdateDistanceUnitPref(r.Context(), user.UserID, distanceUnitPref); err != nil {
		render("Please choose valid distance units")
		return
	}

	squat, err := parseWeight(r.FormValue("squat"), workout.Squat.DefaultStartWeight)
	if err != nil {
		render("Please enter valid positive starting weights")
		return
	}
	bench, err := parseWeight(r.FormValue("bench"), workout.BenchPress.DefaultStartWeight)
	if err != nil {
		render("Please enter valid positive starting weights")
		return
	}
	row, err := parseWeight(r.FormValue("row"), workout.BarbellRow.DefaultStartWeight)
	if err != nil {
		render("Please enter valid positive starting weights")
		return
	}
	press, err := parseWeight(r.FormValue("press"), workout.OHP.DefaultStartWeight)
	if err != nil {
		render("Please enter valid positive starting weights")
		return
	}
	deadlift, err := parseWeight(r.FormValue("deadlift"), workout.Deadlift.DefaultStartWeight)
	if err != nil {
		render("Please enter valid positive starting weights")
		return
	}

	if err := h.progression.SeedInitialProgress(r.Context(), user.UserID, map[string]float64{
		workout.Squat.Name:      squat,
		workout.BenchPress.Name: bench,
		workout.BarbellRow.Name: row,
		workout.OHP.Name:        press,
		workout.Deadlift.Name:   deadlift,
	}); err != nil {
		http.Error(w, "Failed to save onboarding", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/workouts", http.StatusFound)
}

// Dashboard handles GET /
func (h *AuthHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	initialized, err := h.progression.IsInitialized(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load setup state", http.StatusInternalServerError)
		return
	}
	if !initialized {
		http.Redirect(w, r, "/onboarding", http.StatusFound)
		return
	}

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

	unitPref, err := h.authService.GetUnitPref(r.Context(), user.UserID)
	if err != nil {
		http.Error(w, "Failed to load unit preference", http.StatusInternalServerError)
		return
	}
	weightUnit := "lb"
	if unitPref == auth.UnitPrefMetric {
		weightUnit = "kg"
	}

	w.Header().Set("Content-Type", "text/html")
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
	})
}
