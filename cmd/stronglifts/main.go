package main

import (
	"fmt"
	"log"
	"net/http"
	"stronglifts/internal/auth"
	"stronglifts/internal/db"
	"stronglifts/internal/handlers"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Initialize database
	database, err := db.New("stronglifts.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Create schema
	if err := database.CreateSchema(); err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}

	// Initialize session store
	sessionStore := auth.NewSessionStore(database.Conn())
	if err := sessionStore.CleanupExpired(); err != nil {
		log.Fatalf("Failed to clean up expired sessions: %v", err)
	}

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Auth routes
	authHandlers := handlers.NewAuthHandlers(database, sessionStore)
	r.Get("/register", authHandlers.Register)
	r.Post("/register", authHandlers.Register)
	r.Get("/login", authHandlers.Login)
	r.Post("/login", authHandlers.Login)
	r.Post("/logout", authHandlers.Logout)

	// Protected routes (with middleware)
	r.Group(func(r chi.Router) {
		r.Use(auth.SessionMiddleware(sessionStore))
		r.Get("/onboarding", authHandlers.Onboarding)
		r.Post("/onboarding", authHandlers.Onboarding)
		r.Get("/workouts", authHandlers.Dashboard)
		r.Get("/", authHandlers.Dashboard)
		r.Get("/profile", authHandlers.Profile)
		r.Post("/profile", authHandlers.Profile)
		r.Post("/account/delete", authHandlers.DeleteAccount)

		// Workout routes
		workoutHandlers := handlers.NewWorkoutHandlers(database)
		r.Get("/workout/next", workoutHandlers.NextWorkout)
		r.Get("/workout/standalone/new", workoutHandlers.StandaloneEditor)
		r.Post("/workout/standalone", workoutHandlers.CreateStandaloneWorkout)
		r.Post("/workout/standalone/delete-all", workoutHandlers.DeleteAllStandaloneWorkouts)
		r.Get("/workout/standalone/{id}/edit", workoutHandlers.EditStandaloneWorkout)
		r.Post("/workout/standalone/{id}", workoutHandlers.UpdateStandaloneWorkout)
		r.Post("/workout/standalone/{id}/delete", workoutHandlers.DeleteStandaloneWorkout)
		r.Get("/progress/charts", workoutHandlers.ProgressCharts)
		r.Get("/workout/{id}", workoutHandlers.WorkoutPage)
		r.Post("/workout/start", workoutHandlers.StartWorkout)
		r.Post("/workout/finish-open", workoutHandlers.FinishOpenWorkouts)
		r.Post("/workout/{id}/save", workoutHandlers.SaveTracking)
		r.Post("/workout/{id}/exercise/add", workoutHandlers.AddExercise)
		r.Post("/workout/{id}/exercise/{group}/set/add", workoutHandlers.AddSetToExercise)
		r.Post("/workout/{id}/exercise/reorder", workoutHandlers.ReorderExercises)
		r.Delete("/workout/{id}/exercise/{group}", workoutHandlers.DeleteExercise)
		r.Delete("/workout/{id}/set/{n}", workoutHandlers.DeleteSet)
		r.Post("/workout/{id}/set/{n}/complete", workoutHandlers.CompleteSet)
		r.Post("/workout/{id}/finish", workoutHandlers.FinishWorkout)
		r.Delete("/workout/{id}", workoutHandlers.DeleteWorkout)
		r.Post("/progress/{exercise}/deload", workoutHandlers.DeloadExercise)
		r.Post("/progress/{exercise}/skip-increment", workoutHandlers.ToggleSkipIncrement)
		r.Get("/backup/export", workoutHandlers.ExportBackup)
		r.Post("/backup/import", workoutHandlers.ImportBackup)
	})

	// Serve static assets and templates
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	port := 3000
	fmt.Printf("Server running on http://localhost:%d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), r))
}
