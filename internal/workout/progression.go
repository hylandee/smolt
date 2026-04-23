package workout

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// LiftProgress holds the current progression state for one exercise
type LiftProgress struct {
	UserID        int
	ExerciseName  string
	CurrentWeight float64
	IncrementBy   float64
	FailStreak    int
}

// ProgressionService handles workout progression logic
type ProgressionService struct {
	db *sql.DB
}

type FinishSummary struct {
	SessionID       int64
	WorkoutName     string
	CompletedSets   int
	TotalSets       int
	StrengthVolume  float64
	CardioMinutes   int
	CardioMiles     float64
	PersonalRecords []string
}

type ProgressionOverride struct {
	UserID            int
	ExerciseName      string
	SkipNextIncrement bool
}

type BackupData struct {
	Version            int                       `json:"version"`
	ExportedAt         time.Time                 `json:"exportedAt"`
	LiftProgress       []BackupLiftProgress      `json:"liftProgress"`
	Sessions           []BackupSession           `json:"sessions"`
	StandaloneWorkouts []BackupStandaloneWorkout `json:"standaloneWorkouts"`
}

type BackupLiftProgress struct {
	ExerciseName  string  `json:"exerciseName"`
	CurrentWeight float64 `json:"currentWeight"`
	IncrementBy   float64 `json:"incrementBy"`
	FailStreak    int     `json:"failStreak"`
}

type BackupSession struct {
	WorkoutName string             `json:"workoutName"`
	CreatedAt   time.Time          `json:"createdAt"`
	FinishedAt  *time.Time         `json:"finishedAt"`
	Notes       string             `json:"notes"`
	Sets        []BackupSessionSet `json:"sets"`
}

type BackupSessionSet struct {
	ExerciseName string  `json:"exerciseName"`
	SetNumber    int     `json:"setNumber"`
	TargetReps   int     `json:"targetReps"`
	ActualReps   int     `json:"actualReps"`
	Weight       float64 `json:"weight"`
	Completed    bool    `json:"completed"`
}

type BackupStandaloneWorkout struct {
	Title     string                 `json:"title"`
	Notes     string                 `json:"notes"`
	CreatedAt time.Time              `json:"createdAt"`
	Items     []BackupStandaloneItem `json:"items"`
}

type BackupStandaloneItem struct {
	Position      int                          `json:"position"`
	ExerciseName  string                       `json:"exerciseName"`
	ExerciseType  string                       `json:"exerciseType"`
	Sets          int                          `json:"sets"`
	TargetReps    int                          `json:"targetReps"`
	Weight        float64                      `json:"weight"`
	TimeMinutes   int                          `json:"timeMinutes"`
	DistanceMiles float64                      `json:"distanceMiles"`
	SetScheme     []StandaloneStrengthSetInput `json:"setScheme"`
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const (
	unitPrefMetric   = "kg_cm"
	unitPrefImperial = "lb_in"
)

func NewProgressionService(db *sql.DB) *ProgressionService {
	return &ProgressionService{db: db}
}

func normalizeUnitPref(unitPref string) string {
	switch strings.ToLower(strings.TrimSpace(unitPref)) {
	case unitPrefMetric, "kg", "metric", "kg/cm":
		return unitPrefMetric
	case unitPrefImperial, "lb", "imperial", "lb/in":
		return unitPrefImperial
	default:
		return unitPrefImperial
	}
}

func (s *ProgressionService) getUserUnitPref(ctx context.Context, q queryRower, userID int) (string, error) {
	var unitPref string
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(unit_pref, ?) FROM users WHERE id = ?`,
		unitPrefImperial, userID,
	).Scan(&unitPref)
	if err != nil {
		return "", fmt.Errorf("query user unit preference: %w", err)
	}
	return normalizeUnitPref(unitPref), nil
}

func progressionIncrementForExercise(ex Exercise, unitPref string) float64 {
	if normalizeUnitPref(unitPref) == unitPrefImperial {
		return ex.IncrementBy * 2
	}
	return ex.IncrementBy
}

func progressionIncrementForExerciseName(exerciseName, unitPref string) float64 {
	for _, ex := range AllExercises() {
		if ex.Name == exerciseName {
			return progressionIncrementForExercise(ex, unitPref)
		}
	}
	return 0
}

func deloadRoundingStep(unitPref string) float64 {
	if normalizeUnitPref(unitPref) == unitPrefImperial {
		return 5.0
	}
	return 2.5
}

// IsInitialized reports whether the user has seeded lift progress.
func (s *ProgressionService) IsInitialized(ctx context.Context, userID int) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM lift_progress WHERE user_id = ?`,
		userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check lift_progress init: %w", err)
	}
	return count > 0, nil
}

// SeedInitialProgress creates/updates all tracked lifts for onboarding.
func (s *ProgressionService) SeedInitialProgress(ctx context.Context, userID int, startWeights map[string]float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	unitPref, err := s.getUserUnitPref(ctx, tx, userID)
	if err != nil {
		return err
	}

	for _, ex := range AllExercises() {
		w := ex.DefaultStartWeight
		if candidate, ok := startWeights[ex.Name]; ok && candidate > 0 {
			w = candidate
		}
		incrementBy := progressionIncrementForExercise(ex, unitPref)
		_, err := tx.ExecContext(ctx,
			`INSERT INTO lift_progress (user_id, exercise_name, current_weight, increment_by, fail_streak)
			 VALUES (?, ?, ?, ?, 0)
			 ON CONFLICT(user_id, exercise_name)
			 DO UPDATE SET current_weight=excluded.current_weight, increment_by=excluded.increment_by`,
			userID, ex.Name, w, incrementBy,
		)
		if err != nil {
			return fmt.Errorf("seed progress for %s: %w", ex.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// GetOrInitProgress returns current progress for an exercise, creating a default row if absent
func (s *ProgressionService) GetOrInitProgress(ctx context.Context, userID int, ex Exercise) (*LiftProgress, error) {
	var p LiftProgress
	unitPref, err := s.getUserUnitPref(ctx, s.db, userID)
	if err != nil {
		return nil, err
	}
	incrementBy := progressionIncrementForExercise(ex, unitPref)
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id, exercise_name, current_weight, increment_by, fail_streak
		 FROM lift_progress WHERE user_id = ? AND exercise_name = ?`,
		userID, ex.Name,
	).Scan(&p.UserID, &p.ExerciseName, &p.CurrentWeight, &p.IncrementBy, &p.FailStreak)

	if err == sql.ErrNoRows {
		// Initialise with exercise defaults
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO lift_progress (user_id, exercise_name, current_weight, increment_by, fail_streak)
			 VALUES (?, ?, ?, ?, 0)`,
			userID, ex.Name, ex.DefaultStartWeight, incrementBy,
		)
		if err != nil {
			return nil, fmt.Errorf("init lift_progress: %w", err)
		}
		return &LiftProgress{
			UserID:        userID,
			ExerciseName:  ex.Name,
			CurrentWeight: ex.DefaultStartWeight,
			IncrementBy:   incrementBy,
			FailStreak:    0,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query lift_progress: %w", err)
	}
	if p.IncrementBy != incrementBy {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE lift_progress SET increment_by = ?, updated_at = ? WHERE user_id = ? AND exercise_name = ?`,
			incrementBy, time.Now(), userID, ex.Name,
		); err != nil {
			return nil, fmt.Errorf("sync increment_by for %s: %w", ex.Name, err)
		}
		p.IncrementBy = incrementBy
	}
	return &p, nil
}

// NextWorkoutPlan returns the next Program and per-exercise weights for a user
func (s *ProgressionService) NextWorkoutPlan(ctx context.Context, userID int) (Program, map[string]float64, error) {
	// Find the last finished workout
	var lastProgramName string
	err := s.db.QueryRowContext(ctx,
		`SELECT workout_name FROM workout_sessions
		 WHERE user_id = ? AND finished_at IS NOT NULL
		 ORDER BY finished_at DESC LIMIT 1`,
		userID,
	).Scan(&lastProgramName)
	if err != nil && err != sql.ErrNoRows {
		return Program{}, nil, fmt.Errorf("query last workout: %w", err)
	}

	program := NextProgram(lastProgramName)

	weights := make(map[string]float64, len(program.Exercises))
	for _, ex := range program.Exercises {
		p, err := s.GetOrInitProgress(ctx, userID, ex)
		if err != nil {
			return Program{}, nil, err
		}
		weights[ex.Name] = p.CurrentWeight
	}

	return program, weights, nil
}

// StartSession creates a workout_session row and pre-populates exercise_sets
func (s *ProgressionService) StartSession(ctx context.Context, userID int, program Program, weights map[string]float64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_name) VALUES (?, ?)`,
		userID, program.Name,
	)
	if err != nil {
		return 0, fmt.Errorf("insert session: %w", err)
	}
	sessionID, _ := res.LastInsertId()

	globalSetNum := 0
	for _, ex := range program.Exercises {
		w := weights[ex.Name]
		setCount := setCountForExercise(ex)
		for setNum := 1; setNum <= setCount; setNum++ {
			globalSetNum++
			_, err = tx.ExecContext(ctx,
				`INSERT INTO exercise_sets (session_id, exercise_name, set_number, target_reps, weight)
				 VALUES (?, ?, ?, ?, ?)`,
				sessionID, ex.Name, globalSetNum, TargetReps, w,
			)
			if err != nil {
				return 0, fmt.Errorf("insert exercise_set: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return sessionID, nil
}

// CompleteSet records the actual reps and weight for a set
func (s *ProgressionService) CompleteSet(ctx context.Context, sessionID int64, setNumber int, actualReps int, weight float64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE exercise_sets
		 SET actual_reps = ?, weight = ?, completed = 1
		 WHERE session_id = ? AND set_number = ?`,
		actualReps, weight, sessionID, setNumber,
	)
	if err != nil {
		return fmt.Errorf("complete set: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("set %d not found in session %d", setNumber, sessionID)
	}
	return nil
}

// ProgressUpdate describes what changed for one exercise after finishing
type ProgressUpdate struct {
	ExerciseName string
	OldWeight    float64
	NewWeight    float64
	Action       string // "increased", "deload", "unchanged"
}

const (
	StandaloneTypeStrength   = "strength"
	StandaloneTypeTreadmill  = "treadmill"
	StandaloneTypeStaircase  = "staircase"
	StandaloneTypeBike       = "bike"
	StandaloneTypeElliptical = "elliptical"
)

type StandaloneItemInput struct {
	ExerciseName  string
	ExerciseType  string
	Sets          int
	TargetReps    int
	Weight        float64
	SetScheme     []StandaloneStrengthSetInput
	TimeMinutes   int
	DistanceMiles float64
}

type StandaloneStrengthSetInput struct {
	Position   int
	TargetReps int
	Weight     float64
}

type StandaloneWorkoutSummary struct {
	ID        int64
	Title     string
	Notes     string
	ItemCount int
	CreatedAt time.Time
}

type StandaloneWorkoutItemDetail struct {
	ExerciseName  string
	ExerciseType  string
	SetScheme     []StandaloneStrengthSetInput
	TimeMinutes   int
	DistanceMiles float64
}

type StandaloneWorkoutDetail struct {
	ID    int64
	Title string
	Notes string
	Items []StandaloneWorkoutItemDetail
}

// ProgramPlan returns a specific program and current user weights for its exercises.
func (s *ProgressionService) ProgramPlan(ctx context.Context, userID int, programName string) (Program, map[string]float64, error) {
	program, ok := ProgramByName(programName)
	if !ok {
		return Program{}, nil, fmt.Errorf("unknown workout program: %s", programName)
	}

	weights := make(map[string]float64, len(program.Exercises))
	for _, ex := range program.Exercises {
		p, err := s.GetOrInitProgress(ctx, userID, ex)
		if err != nil {
			return Program{}, nil, err
		}
		weights[ex.Name] = p.CurrentWeight
	}

	return program, weights, nil
}

func (s *ProgressionService) ListStandaloneWorkouts(ctx context.Context, userID int) ([]StandaloneWorkoutSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, COALESCE(w.title, ''), COALESCE(w.notes, ''), COUNT(i.id), w.created_at
		 FROM standalone_workouts w
		 LEFT JOIN standalone_workout_items i ON i.workout_id = w.id
		 WHERE w.user_id = ?
		 GROUP BY w.id, w.title, w.notes, w.created_at
		 ORDER BY w.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list standalone workouts: %w", err)
	}
	defer rows.Close()

	workouts := make([]StandaloneWorkoutSummary, 0)
	for rows.Next() {
		var w StandaloneWorkoutSummary
		if err := rows.Scan(&w.ID, &w.Title, &w.Notes, &w.ItemCount, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan standalone workout: %w", err)
		}
		workouts = append(workouts, w)
	}
	return workouts, rows.Err()
}

func (s *ProgressionService) GetStandaloneWorkout(ctx context.Context, userID int, workoutID int64) (*StandaloneWorkoutDetail, error) {
	var detail StandaloneWorkoutDetail
	detail.ID = workoutID
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(title, ''), COALESCE(notes, '')
		 FROM standalone_workouts
		 WHERE id = ? AND user_id = ?`,
		workoutID, userID,
	).Scan(&detail.Title, &detail.Notes)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("standalone workout not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load standalone workout: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, exercise_name, exercise_type, sets, target_reps, weight, time_minutes, distance_miles
		 FROM standalone_workout_items
		 WHERE workout_id = ?
		 ORDER BY position`,
		workoutID,
	)
	if err != nil {
		return nil, fmt.Errorf("load standalone workout items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var itemID int64
		var item StandaloneWorkoutItemDetail
		var sets sql.NullInt64
		var targetReps sql.NullInt64
		var weight sql.NullFloat64
		var timeMinutes sql.NullInt64
		var distanceMiles sql.NullFloat64
		if err := rows.Scan(&itemID, &item.ExerciseName, &item.ExerciseType, &sets, &targetReps, &weight, &timeMinutes, &distanceMiles); err != nil {
			return nil, fmt.Errorf("scan standalone workout item: %w", err)
		}

		if item.ExerciseType == StandaloneTypeStrength {
			setRows, err := s.db.QueryContext(ctx,
				`SELECT position, target_reps, weight
				 FROM standalone_workout_item_sets
				 WHERE workout_item_id = ?
				 ORDER BY position`,
				itemID,
			)
			if err != nil {
				return nil, fmt.Errorf("load standalone workout item set scheme: %w", err)
			}
			for setRows.Next() {
				var setInput StandaloneStrengthSetInput
				if err := setRows.Scan(&setInput.Position, &setInput.TargetReps, &setInput.Weight); err != nil {
					setRows.Close()
					return nil, fmt.Errorf("scan standalone workout item set: %w", err)
				}
				item.SetScheme = append(item.SetScheme, setInput)
			}
			if err := setRows.Err(); err != nil {
				setRows.Close()
				return nil, fmt.Errorf("iterate standalone workout item sets: %w", err)
			}
			setRows.Close()

			if len(item.SetScheme) == 0 {
				count := 5
				if sets.Valid && sets.Int64 > 0 {
					count = int(sets.Int64)
				}
				reps := TargetReps
				if targetReps.Valid && targetReps.Int64 > 0 {
					reps = int(targetReps.Int64)
				}
				setWeight := 0.0
				if weight.Valid && weight.Float64 >= 0 {
					setWeight = weight.Float64
				}
				for idx := 0; idx < count; idx++ {
					item.SetScheme = append(item.SetScheme, StandaloneStrengthSetInput{
						Position:   idx + 1,
						TargetReps: reps,
						Weight:     setWeight,
					})
				}
			}
		} else {
			if timeMinutes.Valid {
				item.TimeMinutes = int(timeMinutes.Int64)
			}
			if distanceMiles.Valid {
				item.DistanceMiles = distanceMiles.Float64
			}
		}

		detail.Items = append(detail.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate standalone workout items: %w", err)
	}

	return &detail, nil
}

func (s *ProgressionService) saveStandaloneWorkoutItems(ctx context.Context, tx *sql.Tx, workoutID int64, items []StandaloneItemInput) error {
	for i, item := range items {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO standalone_workout_items (
				workout_id, position, exercise_name, exercise_type, sets, target_reps, weight, time_minutes, distance_miles
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			workoutID,
			i+1,
			item.ExerciseName,
			item.ExerciseType,
			item.Sets,
			item.TargetReps,
			item.Weight,
			item.TimeMinutes,
			item.DistanceMiles,
		)
		if err != nil {
			return fmt.Errorf("insert standalone workout item: %w", err)
		}

		if item.ExerciseType == StandaloneTypeStrength && len(item.SetScheme) > 0 {
			itemID, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("get standalone workout item id: %w", err)
			}
			for idx, setInput := range item.SetScheme {
				position := setInput.Position
				if position <= 0 {
					position = idx + 1
				}
				_, err = tx.ExecContext(ctx,
					`INSERT INTO standalone_workout_item_sets (workout_item_id, position, target_reps, weight)
					 VALUES (?, ?, ?, ?)`,
					itemID,
					position,
					setInput.TargetReps,
					setInput.Weight,
				)
				if err != nil {
					return fmt.Errorf("insert standalone workout item set: %w", err)
				}
			}
		}
	}
	return nil
}

func (s *ProgressionService) deleteStandaloneWorkoutItems(ctx context.Context, tx *sql.Tx, workoutID int64) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM standalone_workout_items WHERE workout_id = ?`,
		workoutID,
	)
	if err != nil {
		return fmt.Errorf("query standalone workout item ids: %w", err)
	}
	defer rows.Close()

	var itemIDs []int64
	for rows.Next() {
		var itemID int64
		if err := rows.Scan(&itemID); err != nil {
			return fmt.Errorf("scan standalone workout item id: %w", err)
		}
		itemIDs = append(itemIDs, itemID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate standalone workout item ids: %w", err)
	}

	for _, itemID := range itemIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM standalone_workout_item_sets WHERE workout_item_id = ?`, itemID); err != nil {
			return fmt.Errorf("delete standalone workout item sets: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM standalone_workout_items WHERE workout_id = ?`, workoutID); err != nil {
		return fmt.Errorf("delete standalone workout items: %w", err)
	}
	return nil
}

func (s *ProgressionService) StartSessionFromStandalone(ctx context.Context, userID int, standaloneWorkoutID int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM standalone_workouts WHERE id = ? AND user_id = ?`,
		standaloneWorkoutID, userID,
	).Scan(&exists)
	if err != nil {
		return 0, fmt.Errorf("verify standalone workout ownership: %w", err)
	}
	if exists == 0 {
		return 0, fmt.Errorf("standalone workout not found")
	}

	var title string
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(title, '') FROM standalone_workouts WHERE id = ? AND user_id = ?`,
		standaloneWorkoutID, userID,
	).Scan(&title)
	if err != nil {
		return 0, fmt.Errorf("load standalone workout title: %w", err)
	}
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("Custom #%d", standaloneWorkoutID)
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_name) VALUES (?, ?)`,
		userID,
		title,
	)
	if err != nil {
		return 0, fmt.Errorf("insert session: %w", err)
	}
	sessionID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get session id: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, exercise_name, exercise_type, sets, target_reps, weight, time_minutes, distance_miles
		 FROM standalone_workout_items
		 WHERE workout_id = ?
		 ORDER BY position`,
		standaloneWorkoutID,
	)
	if err != nil {
		return 0, fmt.Errorf("load standalone workout items: %w", err)
	}
	defer rows.Close()

	globalSetNum := 0
	for rows.Next() {
		var itemID int64
		var exerciseName string
		var exerciseType string
		var sets sql.NullInt64
		var targetReps sql.NullInt64
		var weight sql.NullFloat64
		var timeMinutes sql.NullInt64
		var distanceMiles sql.NullFloat64

		if err := rows.Scan(&itemID, &exerciseName, &exerciseType, &sets, &targetReps, &weight, &timeMinutes, &distanceMiles); err != nil {
			return 0, fmt.Errorf("scan standalone item: %w", err)
		}

		if exerciseName == "" {
			exerciseName = defaultStandaloneExerciseName(exerciseType)
		}

		setCount := 1
		target := TargetReps
		setWeight := 0.0

		switch exerciseType {
		case StandaloneTypeStrength:
			setRows, err := tx.QueryContext(ctx,
				`SELECT target_reps, weight
				 FROM standalone_workout_item_sets
				 WHERE workout_item_id = ?
				 ORDER BY position`,
				itemID,
			)
			if err != nil {
				return 0, fmt.Errorf("load standalone set scheme: %w", err)
			}

			type setSchemeRow struct {
				targetReps int
				weight     float64
			}
			var scheme []setSchemeRow
			for setRows.Next() {
				var row setSchemeRow
				if err := setRows.Scan(&row.targetReps, &row.weight); err != nil {
					setRows.Close()
					return 0, fmt.Errorf("scan standalone set scheme: %w", err)
				}
				scheme = append(scheme, row)
			}
			if err := setRows.Err(); err != nil {
				setRows.Close()
				return 0, fmt.Errorf("iterate standalone set scheme: %w", err)
			}
			setRows.Close()

			if len(scheme) > 0 {
				for _, schemeSet := range scheme {
					globalSetNum++
					_, err = tx.ExecContext(ctx,
						`INSERT INTO exercise_sets (session_id, exercise_name, set_number, target_reps, weight)
						 VALUES (?, ?, ?, ?, ?)`,
						sessionID, exerciseName, globalSetNum, schemeSet.targetReps, schemeSet.weight,
					)
					if err != nil {
						return 0, fmt.Errorf("insert standalone scheme set: %w", err)
					}
				}
				continue
			}

			if sets.Valid && sets.Int64 > 0 {
				setCount = int(sets.Int64)
			} else {
				setCount = SetsPerExercise
			}
			if targetReps.Valid && targetReps.Int64 > 0 {
				target = int(targetReps.Int64)
			}
			if weight.Valid && weight.Float64 >= 0 {
				setWeight = weight.Float64
			}
		default:
			if timeMinutes.Valid && timeMinutes.Int64 > 0 {
				target = int(timeMinutes.Int64)
			} else {
				target = 10
			}
			if (exerciseType == StandaloneTypeTreadmill || exerciseType == StandaloneTypeBike) && distanceMiles.Valid && distanceMiles.Float64 > 0 {
				setWeight = distanceMiles.Float64
			}
		}

		for i := 0; i < setCount; i++ {
			globalSetNum++
			_, err = tx.ExecContext(ctx,
				`INSERT INTO exercise_sets (session_id, exercise_name, set_number, target_reps, weight)
				 VALUES (?, ?, ?, ?, ?)`,
				sessionID, exerciseName, globalSetNum, target, setWeight,
			)
			if err != nil {
				return 0, fmt.Errorf("insert standalone exercise set: %w", err)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate standalone items: %w", err)
	}

	if globalSetNum == 0 {
		return 0, fmt.Errorf("standalone workout has no items")
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit standalone session: %w", err)
	}
	return sessionID, nil
}

func defaultStandaloneExerciseName(exerciseType string) string {
	switch exerciseType {
	case StandaloneTypeTreadmill:
		return "Treadmill"
	case StandaloneTypeStaircase:
		return "Staircase"
	case StandaloneTypeBike:
		return "Exercise Bike"
	case StandaloneTypeElliptical:
		return "Elliptical"
	default:
		return "Custom Exercise"
	}
}

func (s *ProgressionService) SaveStandaloneWorkout(ctx context.Context, userID int, title, notes string, items []StandaloneItemInput) (int64, error) {
	if len(items) == 0 {
		return 0, fmt.Errorf("standalone workout requires at least one item")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO standalone_workouts (user_id, title, notes) VALUES (?, ?, ?)`,
		userID, strings.TrimSpace(title), notes,
	)
	if err != nil {
		return 0, fmt.Errorf("insert standalone workout: %w", err)
	}
	workoutID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get standalone workout id: %w", err)
	}

	if err := s.saveStandaloneWorkoutItems(ctx, tx, workoutID, items); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit standalone workout: %w", err)
	}

	return workoutID, nil
}

func (s *ProgressionService) UpdateStandaloneWorkout(ctx context.Context, userID int, workoutID int64, title, notes string, items []StandaloneItemInput) error {
	if len(items) == 0 {
		return fmt.Errorf("standalone workout requires at least one item")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE standalone_workouts SET title = ?, notes = ? WHERE id = ? AND user_id = ?`,
		strings.TrimSpace(title), notes, workoutID, userID,
	)
	if err != nil {
		return fmt.Errorf("update standalone workout: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("standalone workout not found")
	}

	if err := s.deleteStandaloneWorkoutItems(ctx, tx, workoutID); err != nil {
		return err
	}
	if err := s.saveStandaloneWorkoutItems(ctx, tx, workoutID, items); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit standalone workout update: %w", err)
	}
	return nil
}

func (s *ProgressionService) DeleteStandaloneWorkout(ctx context.Context, userID int, workoutID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.deleteStandaloneWorkoutItems(ctx, tx, workoutID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM standalone_workouts WHERE id = ? AND user_id = ?`, workoutID, userID)
	if err != nil {
		return fmt.Errorf("delete standalone workout: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("standalone workout not found")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete standalone workout: %w", err)
	}
	return nil
}

func (s *ProgressionService) DeleteAllStandaloneWorkouts(ctx context.Context, userID int) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM standalone_workouts WHERE user_id = ?`, userID)
	if err != nil {
		return 0, fmt.Errorf("query standalone workouts for delete-all: %w", err)
	}
	defer rows.Close()

	var workoutIDs []int64
	for rows.Next() {
		var workoutID int64
		if err := rows.Scan(&workoutID); err != nil {
			return 0, fmt.Errorf("scan standalone workout id: %w", err)
		}
		workoutIDs = append(workoutIDs, workoutID)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate standalone workout ids: %w", err)
	}

	deleted := 0
	for _, workoutID := range workoutIDs {
		if err := s.DeleteStandaloneWorkout(ctx, userID, workoutID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

type SetUpdate struct {
	SetNumber  int
	ActualReps int
	Weight     float64
	Completed  bool
}

func (s *ProgressionService) ApplySetUpdates(ctx context.Context, sessionID int64, updates []SetUpdate) error {
	for _, u := range updates {
		if u.SetNumber <= 0 {
			continue
		}
		completed := 0
		if u.Completed {
			completed = 1
		}
		_, err := s.db.ExecContext(ctx,
			`UPDATE exercise_sets
			 SET actual_reps = ?, weight = ?, completed = ?
			 WHERE session_id = ? AND set_number = ?`,
			u.ActualReps, u.Weight, completed, sessionID, u.SetNumber,
		)
		if err != nil {
			return fmt.Errorf("apply set update: %w", err)
		}
	}
	return nil
}

func (s *ProgressionService) AddExerciseToSession(ctx context.Context, sessionID int64, exerciseType, exerciseName string, sets, targetReps int, weight float64) error {
	if sets <= 0 {
		sets = 1
	}
	if targetReps <= 0 {
		targetReps = TargetReps
	}

	var nextSetNum int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(set_number), 0) + 1 FROM exercise_sets WHERE session_id = ?`,
		sessionID,
	).Scan(&nextSetNum)
	if err != nil {
		return fmt.Errorf("query next set number: %w", err)
	}

	if exerciseName == "" {
		exerciseName = defaultStandaloneExerciseName(exerciseType)
	}

	canonicalName := canonicalExerciseName(exerciseName)
	storedExerciseName := strings.TrimSpace(exerciseName)

	var lastExerciseName string
	_ = s.db.QueryRowContext(ctx,
		`SELECT exercise_name FROM exercise_sets WHERE session_id = ? ORDER BY set_number DESC LIMIT 1`,
		sessionID,
	).Scan(&lastExerciseName)
	if canonicalExerciseName(lastExerciseName) == canonicalName {
		rows, err := s.db.QueryContext(ctx,
			`SELECT exercise_name FROM exercise_sets WHERE session_id = ?`,
			sessionID,
		)
		if err != nil {
			return fmt.Errorf("query existing exercise names: %w", err)
		}
		defer rows.Close()

		instance := 1
		for rows.Next() {
			var existing string
			if err := rows.Scan(&existing); err != nil {
				return fmt.Errorf("scan exercise name: %w", err)
			}
			if canonicalExerciseName(existing) == canonicalName {
				instance++
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate exercise names: %w", err)
		}
		storedExerciseName = withExerciseInstance(canonicalName, instance)
	}

	for i := 0; i < sets; i++ {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO exercise_sets (session_id, exercise_name, set_number, target_reps, weight)
			 VALUES (?, ?, ?, ?, ?)`,
			sessionID, storedExerciseName, nextSetNum+i, targetReps, weight,
		)
		if err != nil {
			return fmt.Errorf("insert added exercise set: %w", err)
		}
	}

	return nil
}

// FinishSession marks the session finished and applies progression logic
func (s *ProgressionService) FinishSession(ctx context.Context, sessionID int64, userID int) ([]ProgressUpdate, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var workoutName string
	var alreadyFinished bool
	err = tx.QueryRowContext(ctx,
		`SELECT workout_name, finished_at IS NOT NULL FROM workout_sessions WHERE id = ? AND user_id = ?`,
		sessionID, userID,
	).Scan(&workoutName, &alreadyFinished)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load session before finish: %w", err)
	}

	// Mark session finished
	_, err = tx.ExecContext(ctx,
		`UPDATE workout_sessions SET finished_at = ? WHERE id = ?`,
		time.Now(), sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("finish session: %w", err)
	}

	if alreadyFinished {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit finished session: %w", err)
		}
		return nil, nil
	}

	isStandardProgressionWorkout := workoutName == "A" || workoutName == "B"
	if !isStandardProgressionWorkout {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit custom session: %w", err)
		}
		return nil, nil
	}

	var incompleteCount int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM exercise_sets WHERE session_id = ? AND COALESCE(completed, 0) = 0`,
		sessionID,
	).Scan(&incompleteCount)
	if err != nil {
		return nil, fmt.Errorf("check incomplete sets: %w", err)
	}
	if incompleteCount > 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit incomplete session: %w", err)
		}
		return nil, nil
	}

	unitPref, err := s.getUserUnitPref(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	deloadStep := deloadRoundingStep(unitPref)

	// Get all sets grouped by exercise
	rows, err := tx.QueryContext(ctx,
		`SELECT exercise_name, set_number, target_reps, COALESCE(actual_reps, 0)
		 FROM exercise_sets WHERE session_id = ? ORDER BY exercise_name, set_number`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query sets: %w", err)
	}
	defer rows.Close()

	// Group reps per exercise
	type setResult struct{ target, actual int }
	exerciseSets := make(map[string][]setResult)
	for rows.Next() {
		var exName string
		var setNum, target, actual int
		if err := rows.Scan(&exName, &setNum, &target, &actual); err != nil {
			return nil, err
		}
		canonical := canonicalExerciseName(exName)
		exerciseSets[canonical] = append(exerciseSets[canonical], setResult{target, actual})
	}
	rows.Close()

	var updates []ProgressUpdate

	for exName, sets := range exerciseSets {
		// Check if all sets hit target reps
		allComplete := true
		for _, s := range sets {
			if s.actual < s.target {
				allComplete = false
				break
			}
		}

		// Fetch current progress
		var p LiftProgress
		err := tx.QueryRowContext(ctx,
			`SELECT current_weight, increment_by, fail_streak FROM lift_progress
			 WHERE user_id = ? AND exercise_name = ?`,
			userID, exName,
		).Scan(&p.CurrentWeight, &p.IncrementBy, &p.FailStreak)
		if err == sql.ErrNoRows {
			continue // exercise not tracked yet
		}
		if err != nil {
			return nil, fmt.Errorf("query progress for %s: %w", exName, err)
		}

		expectedIncrement := progressionIncrementForExerciseName(exName, unitPref)
		if expectedIncrement > 0 {
			p.IncrementBy = expectedIncrement
		}

		skipIncrement := false
		var skipIncrementRaw int
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(skip_next_increment, 0) FROM progression_overrides WHERE user_id = ? AND exercise_name = ?`,
			userID, exName,
		).Scan(&skipIncrementRaw); err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("query progression override for %s: %w", exName, err)
		}
		skipIncrement = skipIncrementRaw == 1

		oldWeight := p.CurrentWeight
		var newWeight float64
		action := "unchanged"

		if allComplete {
			if skipIncrement {
				newWeight = p.CurrentWeight
				action = "unchanged"
			} else {
				newWeight = p.CurrentWeight + p.IncrementBy
				action = "increased"
			}
			p.FailStreak = 0
		} else {
			p.FailStreak++
			if p.FailStreak >= 3 {
				// Deload: round down to the nearest unit-appropriate step.
				newWeight = math.Floor((p.CurrentWeight*0.9)/deloadStep) * deloadStep
				action = "deload"
				p.FailStreak = 0
			} else {
				newWeight = p.CurrentWeight
			}
		}

		_, err = tx.ExecContext(ctx,
			`UPDATE lift_progress SET current_weight = ?, increment_by = ?, fail_streak = ?, updated_at = ?
			 WHERE user_id = ? AND exercise_name = ?`,
			newWeight, p.IncrementBy, p.FailStreak, time.Now(), userID, exName,
		)
		if err != nil {
			return nil, fmt.Errorf("update progress for %s: %w", exName, err)
		}

		if skipIncrement {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO progression_overrides (user_id, exercise_name, skip_next_increment)
				 VALUES (?, ?, 0)
				 ON CONFLICT(user_id, exercise_name)
				 DO UPDATE SET skip_next_increment = 0`,
				userID, exName,
			); err != nil {
				return nil, fmt.Errorf("clear skip increment override for %s: %w", exName, err)
			}
		}

		updates = append(updates, ProgressUpdate{
			ExerciseName: exName,
			OldWeight:    oldWeight,
			NewWeight:    newWeight,
			Action:       action,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return updates, nil
}

// DeleteSession removes a session and all its sets
func (s *ProgressionService) DeleteSession(ctx context.Context, sessionID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `DELETE FROM exercise_sets WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete sets: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM workout_sessions WHERE id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return tx.Commit()
}

// SessionSet represents a single set row for UI rendering
type SessionSet struct {
	Number      int
	LocalNumber int
	Weight      float64
	TargetReps  int
	ActualReps  int
	Completed   bool
}

// SessionExercise groups sets by exercise name for UI rendering
type SessionExercise struct {
	Index  int
	Name   string
	Weight float64
	Sets   []SessionSet
}

// SessionView holds everything the workout page needs
type SessionView struct {
	ID          int64
	ProgramName string
	Finished    bool
	Exercises   []SessionExercise
}

// GetSession loads a session and all its sets, organised by contiguous exercise blocks.
// This allows multiple instances of the same exercise in a single workout session.
func (s *ProgressionService) GetSession(ctx context.Context, sessionID int64) (*SessionView, error) {
	var view SessionView
	view.ID = sessionID

	err := s.db.QueryRowContext(ctx,
		`SELECT workout_name, finished_at IS NOT NULL FROM workout_sessions WHERE id = ?`, sessionID,
	).Scan(&view.ProgramName, &view.Finished)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT exercise_name, set_number, weight, target_reps, COALESCE(actual_reps, 0), COALESCE(completed, 0)
		 FROM exercise_sets WHERE session_id = ? ORDER BY set_number`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query sets: %w", err)
	}
	defer rows.Close()

	currentGroup := -1
	lastExercise := ""
	for rows.Next() {
		var exKey string
		var set SessionSet
		if err := rows.Scan(&exKey, &set.Number, &set.Weight, &set.TargetReps, &set.ActualReps, &set.Completed); err != nil {
			return nil, err
		}
		exName := canonicalExerciseName(exKey)

		// Start a new exercise block whenever the exercise name changes in set order.
		if currentGroup == -1 || exKey != lastExercise {
			currentGroup = len(view.Exercises)
			view.Exercises = append(view.Exercises, SessionExercise{
				Index:  currentGroup + 1,
				Name:   exName,
				Weight: set.Weight,
			})
			lastExercise = exKey
		}

		set.LocalNumber = len(view.Exercises[currentGroup].Sets) + 1
		view.Exercises[currentGroup].Sets = append(view.Exercises[currentGroup].Sets, set)
	}
	return &view, rows.Err()
}

type sessionSetRow struct {
	ExerciseKey  string
	ExerciseName string
	TargetReps   int
	Weight       float64
	ActualReps   int
	Completed    bool
}

const exerciseInstanceDelimiter = " @@"

func canonicalExerciseName(name string) string {
	trimmed := strings.TrimSpace(name)
	if idx := strings.Index(trimmed, exerciseInstanceDelimiter); idx >= 0 {
		return strings.TrimSpace(trimmed[:idx])
	}
	return trimmed
}

func withExerciseInstance(name string, instance int) string {
	if instance <= 1 {
		return strings.TrimSpace(name)
	}
	return fmt.Sprintf("%s%s%d", strings.TrimSpace(name), exerciseInstanceDelimiter, instance)
}

func (s *ProgressionService) loadSessionExerciseGroups(ctx context.Context, tx *sql.Tx, sessionID int64) ([][]sessionSetRow, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT exercise_name, target_reps, weight, COALESCE(actual_reps, 0), COALESCE(completed, 0)
		 FROM exercise_sets WHERE session_id = ? ORDER BY set_number`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query sets for grouping: %w", err)
	}
	defer rows.Close()

	groups := make([][]sessionSetRow, 0)
	lastExercise := ""
	for rows.Next() {
		var row sessionSetRow
		if err := rows.Scan(&row.ExerciseKey, &row.TargetReps, &row.Weight, &row.ActualReps, &row.Completed); err != nil {
			return nil, fmt.Errorf("scan grouped set row: %w", err)
		}
		row.ExerciseName = canonicalExerciseName(row.ExerciseKey)
		if len(groups) == 0 || row.ExerciseKey != lastExercise {
			groups = append(groups, make([]sessionSetRow, 0, 8))
			lastExercise = row.ExerciseKey
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate grouped set rows: %w", err)
	}
	return groups, nil
}

func (s *ProgressionService) rewriteSessionGroups(ctx context.Context, tx *sql.Tx, sessionID int64, groups [][]sessionSetRow) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM exercise_sets WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete old sets for rewrite: %w", err)
	}

	setNum := 0
	for _, group := range groups {
		for _, row := range group {
			setNum++
			completed := 0
			if row.Completed {
				completed = 1
			}
			exerciseKey := strings.TrimSpace(row.ExerciseKey)
			if exerciseKey == "" {
				exerciseKey = strings.TrimSpace(row.ExerciseName)
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO exercise_sets (session_id, exercise_name, set_number, target_reps, weight, actual_reps, completed)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				sessionID,
				exerciseKey,
				setNum,
				row.TargetReps,
				row.Weight,
				row.ActualReps,
				completed,
			)
			if err != nil {
				return fmt.Errorf("insert reordered set: %w", err)
			}
		}
	}

	return nil
}

func (s *ProgressionService) DeleteExerciseGroup(ctx context.Context, sessionID int64, groupIndex int) error {
	if groupIndex < 1 {
		return fmt.Errorf("invalid exercise group index")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	groups, err := s.loadSessionExerciseGroups(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if groupIndex > len(groups) {
		return fmt.Errorf("exercise group not found")
	}

	updated := make([][]sessionSetRow, 0, len(groups)-1)
	for i, group := range groups {
		if i == groupIndex-1 {
			continue
		}
		updated = append(updated, group)
	}

	if err := s.rewriteSessionGroups(ctx, tx, sessionID, updated); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete exercise group: %w", err)
	}
	return nil
}

func (s *ProgressionService) ReorderExerciseGroups(ctx context.Context, sessionID int64, fromIndex, toIndex int) error {
	if fromIndex < 1 || toIndex < 1 {
		return fmt.Errorf("invalid exercise group index")
	}
	if fromIndex == toIndex {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	groups, err := s.loadSessionExerciseGroups(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if fromIndex > len(groups) || toIndex > len(groups) {
		return fmt.Errorf("exercise group not found")
	}

	from := fromIndex - 1
	to := toIndex - 1
	group := groups[from]
	groups = append(groups[:from], groups[from+1:]...)
	if to >= len(groups) {
		groups = append(groups, group)
	} else {
		groups = append(groups[:to], append([][]sessionSetRow{group}, groups[to:]...)...)
	}

	if err := s.rewriteSessionGroups(ctx, tx, sessionID, groups); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reorder exercise groups: %w", err)
	}
	return nil
}

func (s *ProgressionService) AddSetToExerciseGroup(ctx context.Context, sessionID int64, groupIndex int, targetReps int, weight float64) error {
	if groupIndex < 1 {
		return fmt.Errorf("invalid exercise group index")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	groups, err := s.loadSessionExerciseGroups(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if groupIndex > len(groups) {
		return fmt.Errorf("exercise group not found")
	}

	group := groups[groupIndex-1]
	if len(group) == 0 {
		return fmt.Errorf("exercise group is empty")
	}
	lastSet := group[len(group)-1]

	if targetReps <= 0 {
		targetReps = lastSet.TargetReps
		if targetReps <= 0 {
			targetReps = TargetReps
		}
	}
	if weight < 0 {
		weight = lastSet.Weight
	}

	group = append(group, sessionSetRow{
		ExerciseKey:  lastSet.ExerciseKey,
		ExerciseName: lastSet.ExerciseName,
		TargetReps:   targetReps,
		Weight:       weight,
		ActualReps:   0,
		Completed:    false,
	})
	groups[groupIndex-1] = group

	if err := s.rewriteSessionGroups(ctx, tx, sessionID, groups); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add set: %w", err)
	}
	return nil
}

func (s *ProgressionService) DeleteSet(ctx context.Context, sessionID int64, setNumber int) error {
	if setNumber <= 0 {
		return fmt.Errorf("invalid set number")
	}

	res, err := s.db.ExecContext(ctx,
		`DELETE FROM exercise_sets WHERE session_id = ? AND set_number = ?`,
		sessionID, setNumber,
	)
	if err != nil {
		return fmt.Errorf("delete set: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("set not found")
	}
	return nil
}

func (s *ProgressionService) SetSkipNextIncrement(ctx context.Context, userID int, exerciseName string, skip bool) error {
	exerciseName = canonicalExerciseName(exerciseName)
	if exerciseName == "" {
		return fmt.Errorf("exercise name is required")
	}
	skipValue := 0
	if skip {
		skipValue = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO progression_overrides (user_id, exercise_name, skip_next_increment)
		 VALUES (?, ?, ?)
		 ON CONFLICT(user_id, exercise_name)
		 DO UPDATE SET skip_next_increment = excluded.skip_next_increment`,
		userID, exerciseName, skipValue,
	)
	if err != nil {
		return fmt.Errorf("set skip increment override: %w", err)
	}
	return nil
}

func (s *ProgressionService) DeloadExercise(ctx context.Context, userID int, exerciseName string) error {
	exerciseName = canonicalExerciseName(exerciseName)
	if exerciseName == "" {
		return fmt.Errorf("exercise name is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	unitPref, err := s.getUserUnitPref(ctx, tx, userID)
	if err != nil {
		return err
	}
	deloadStep := deloadRoundingStep(unitPref)

	var currentWeight float64
	if err := tx.QueryRowContext(ctx,
		`SELECT current_weight FROM lift_progress WHERE user_id = ? AND exercise_name = ?`,
		userID, exerciseName,
	).Scan(&currentWeight); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("exercise progress not found")
		}
		return fmt.Errorf("load progress for deload: %w", err)
	}

	newWeight := math.Floor((currentWeight*0.9)/deloadStep) * deloadStep
	_, err = tx.ExecContext(ctx,
		`UPDATE lift_progress
		 SET current_weight = ?, fail_streak = 0, updated_at = ?
		 WHERE user_id = ? AND exercise_name = ?`,
		newWeight, time.Now(), userID, exerciseName,
	)
	if err != nil {
		return fmt.Errorf("update progress deload: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deload: %w", err)
	}
	return nil
}

func isCardioExerciseName(name string) bool {
	switch canonicalExerciseName(name) {
	case "Treadmill", "Exercise Bike", "Staircase", "Elliptical":
		return true
	default:
		return false
	}
}

func (s *ProgressionService) SessionFinishSummary(ctx context.Context, userID int, sessionID int64) (*FinishSummary, error) {
	var workoutName string
	if err := s.db.QueryRowContext(ctx,
		`SELECT workout_name FROM workout_sessions WHERE id = ? AND user_id = ?`,
		sessionID, userID,
	).Scan(&workoutName); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("load session for summary: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT exercise_name, target_reps, COALESCE(actual_reps, 0), weight, COALESCE(completed, 0)
		 FROM exercise_sets
		 WHERE session_id = ?
		 ORDER BY set_number`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("load session sets for summary: %w", err)
	}
	defer rows.Close()

	summary := &FinishSummary{SessionID: sessionID, WorkoutName: workoutName}
	sessionMax := map[string]float64{}

	for rows.Next() {
		var exerciseName string
		var targetReps int
		var actualReps int
		var weight float64
		var completed bool
		if err := rows.Scan(&exerciseName, &targetReps, &actualReps, &weight, &completed); err != nil {
			return nil, fmt.Errorf("scan session set for summary: %w", err)
		}
		summary.TotalSets++
		if !completed {
			continue
		}
		summary.CompletedSets++

		name := canonicalExerciseName(exerciseName)
		if isCardioExerciseName(name) {
			minutes := targetReps
			if actualReps > 0 {
				minutes = actualReps
			}
			summary.CardioMinutes += minutes
			if name == "Treadmill" || name == "Exercise Bike" {
				summary.CardioMiles += weight
			}
			continue
		}

		reps := targetReps
		if actualReps > 0 {
			reps = actualReps
		}
		summary.StrengthVolume += weight * float64(reps)
		if weight > sessionMax[name] {
			sessionMax[name] = weight
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session set for summary: %w", err)
	}

	for exerciseName, weight := range sessionMax {
		var prior sql.NullFloat64
		err := s.db.QueryRowContext(ctx,
			`SELECT MAX(es.weight)
			 FROM exercise_sets es
			 JOIN workout_sessions ws ON ws.id = es.session_id
			 WHERE ws.user_id = ?
			   AND ws.id <> ?
			   AND ws.finished_at IS NOT NULL
			   AND COALESCE(es.completed, 0) = 1
			   AND es.exercise_name = ?`,
			userID, sessionID, exerciseName,
		).Scan(&prior)
		if err != nil {
			return nil, fmt.Errorf("query prior max for %s: %w", exerciseName, err)
		}
		if !prior.Valid || weight > prior.Float64 {
			summary.PersonalRecords = append(summary.PersonalRecords, exerciseName)
		}
	}

	return summary, nil
}

func (s *ProgressionService) ExportBackup(ctx context.Context, userID int) (*BackupData, error) {
	backup := &BackupData{Version: 1, ExportedAt: time.Now()}

	liftRows, err := s.db.QueryContext(ctx,
		`SELECT exercise_name, current_weight, increment_by, fail_streak
		 FROM lift_progress
		 WHERE user_id = ?
		 ORDER BY exercise_name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query lift progress for backup: %w", err)
	}
	for liftRows.Next() {
		var row BackupLiftProgress
		if err := liftRows.Scan(&row.ExerciseName, &row.CurrentWeight, &row.IncrementBy, &row.FailStreak); err != nil {
			liftRows.Close()
			return nil, fmt.Errorf("scan lift progress backup row: %w", err)
		}
		backup.LiftProgress = append(backup.LiftProgress, row)
	}
	if err := liftRows.Err(); err != nil {
		liftRows.Close()
		return nil, fmt.Errorf("iterate lift progress backup rows: %w", err)
	}
	liftRows.Close()

	sessionRows, err := s.db.QueryContext(ctx,
		`SELECT id, workout_name, created_at, finished_at, COALESCE(notes, '')
		 FROM workout_sessions
		 WHERE user_id = ?
		 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query sessions for backup: %w", err)
	}
	for sessionRows.Next() {
		var sessionID int64
		var row BackupSession
		var finishedAt sql.NullTime
		if err := sessionRows.Scan(&sessionID, &row.WorkoutName, &row.CreatedAt, &finishedAt, &row.Notes); err != nil {
			sessionRows.Close()
			return nil, fmt.Errorf("scan session backup row: %w", err)
		}
		if finishedAt.Valid {
			v := finishedAt.Time
			row.FinishedAt = &v
		}

		setRows, err := s.db.QueryContext(ctx,
			`SELECT exercise_name, set_number, target_reps, COALESCE(actual_reps, 0), weight, COALESCE(completed, 0)
			 FROM exercise_sets
			 WHERE session_id = ?
			 ORDER BY set_number`,
			sessionID,
		)
		if err != nil {
			sessionRows.Close()
			return nil, fmt.Errorf("query backup session sets: %w", err)
		}
		for setRows.Next() {
			var setRow BackupSessionSet
			if err := setRows.Scan(&setRow.ExerciseName, &setRow.SetNumber, &setRow.TargetReps, &setRow.ActualReps, &setRow.Weight, &setRow.Completed); err != nil {
				setRows.Close()
				sessionRows.Close()
				return nil, fmt.Errorf("scan backup session set row: %w", err)
			}
			row.Sets = append(row.Sets, setRow)
		}
		if err := setRows.Err(); err != nil {
			setRows.Close()
			sessionRows.Close()
			return nil, fmt.Errorf("iterate backup session sets: %w", err)
		}
		setRows.Close()

		backup.Sessions = append(backup.Sessions, row)
	}
	if err := sessionRows.Err(); err != nil {
		sessionRows.Close()
		return nil, fmt.Errorf("iterate sessions for backup: %w", err)
	}
	sessionRows.Close()

	workoutRows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(title, ''), COALESCE(notes, ''), created_at
		 FROM standalone_workouts
		 WHERE user_id = ?
		 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query standalone workouts for backup: %w", err)
	}
	for workoutRows.Next() {
		var workoutID int64
		var workout BackupStandaloneWorkout
		if err := workoutRows.Scan(&workoutID, &workout.Title, &workout.Notes, &workout.CreatedAt); err != nil {
			workoutRows.Close()
			return nil, fmt.Errorf("scan standalone workout backup row: %w", err)
		}

		itemRows, err := s.db.QueryContext(ctx,
			`SELECT id, position, exercise_name, exercise_type,
			        COALESCE(sets, 0), COALESCE(target_reps, 0), COALESCE(weight, 0),
			        COALESCE(time_minutes, 0), COALESCE(distance_miles, 0)
			 FROM standalone_workout_items
			 WHERE workout_id = ?
			 ORDER BY position`,
			workoutID,
		)
		if err != nil {
			workoutRows.Close()
			return nil, fmt.Errorf("query standalone workout items for backup: %w", err)
		}
		for itemRows.Next() {
			var itemID int64
			var item BackupStandaloneItem
			if err := itemRows.Scan(&itemID, &item.Position, &item.ExerciseName, &item.ExerciseType, &item.Sets, &item.TargetReps, &item.Weight, &item.TimeMinutes, &item.DistanceMiles); err != nil {
				itemRows.Close()
				workoutRows.Close()
				return nil, fmt.Errorf("scan standalone workout item for backup: %w", err)
			}

			schemeRows, err := s.db.QueryContext(ctx,
				`SELECT position, target_reps, weight
				 FROM standalone_workout_item_sets
				 WHERE workout_item_id = ?
				 ORDER BY position`,
				itemID,
			)
			if err != nil {
				itemRows.Close()
				workoutRows.Close()
				return nil, fmt.Errorf("query standalone workout item scheme for backup: %w", err)
			}
			for schemeRows.Next() {
				var scheme StandaloneStrengthSetInput
				if err := schemeRows.Scan(&scheme.Position, &scheme.TargetReps, &scheme.Weight); err != nil {
					schemeRows.Close()
					itemRows.Close()
					workoutRows.Close()
					return nil, fmt.Errorf("scan standalone workout item scheme for backup: %w", err)
				}
				item.SetScheme = append(item.SetScheme, scheme)
			}
			if err := schemeRows.Err(); err != nil {
				schemeRows.Close()
				itemRows.Close()
				workoutRows.Close()
				return nil, fmt.Errorf("iterate standalone workout item schemes for backup: %w", err)
			}
			schemeRows.Close()

			workout.Items = append(workout.Items, item)
		}
		if err := itemRows.Err(); err != nil {
			itemRows.Close()
			workoutRows.Close()
			return nil, fmt.Errorf("iterate standalone workout items for backup: %w", err)
		}
		itemRows.Close()

		backup.StandaloneWorkouts = append(backup.StandaloneWorkouts, workout)
	}
	if err := workoutRows.Err(); err != nil {
		workoutRows.Close()
		return nil, fmt.Errorf("iterate standalone workouts for backup: %w", err)
	}
	workoutRows.Close()

	return backup, nil
}

func (s *ProgressionService) ImportBackup(ctx context.Context, userID int, backup BackupData) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM progression_overrides WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("clear progression overrides: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM exercise_sets WHERE session_id IN (SELECT id FROM workout_sessions WHERE user_id = ?)`,
		userID,
	); err != nil {
		return fmt.Errorf("clear exercise sets for import: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workout_sessions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("clear workout sessions for import: %w", err)
	}

	standaloneRows, err := tx.QueryContext(ctx, `SELECT id FROM standalone_workouts WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("query standalone workouts to clear: %w", err)
	}
	var standaloneIDs []int64
	for standaloneRows.Next() {
		var id int64
		if err := standaloneRows.Scan(&id); err != nil {
			standaloneRows.Close()
			return fmt.Errorf("scan standalone workout id to clear: %w", err)
		}
		standaloneIDs = append(standaloneIDs, id)
	}
	if err := standaloneRows.Err(); err != nil {
		standaloneRows.Close()
		return fmt.Errorf("iterate standalone workout ids to clear: %w", err)
	}
	standaloneRows.Close()

	for _, workoutID := range standaloneIDs {
		if err := s.deleteStandaloneWorkoutItems(ctx, tx, workoutID); err != nil {
			return fmt.Errorf("clear standalone workout %d items: %w", workoutID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM standalone_workouts WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("clear standalone workouts for import: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM lift_progress WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("clear lift progress for import: %w", err)
	}

	for _, row := range backup.LiftProgress {
		if strings.TrimSpace(row.ExerciseName) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO lift_progress (user_id, exercise_name, current_weight, increment_by, fail_streak, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			userID,
			canonicalExerciseName(row.ExerciseName),
			row.CurrentWeight,
			row.IncrementBy,
			row.FailStreak,
			time.Now(),
		); err != nil {
			return fmt.Errorf("import lift progress row: %w", err)
		}
	}

	for _, session := range backup.Sessions {
		createdAt := session.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO workout_sessions (user_id, workout_name, created_at, finished_at, notes)
			 VALUES (?, ?, ?, ?, ?)`,
			userID,
			session.WorkoutName,
			createdAt,
			session.FinishedAt,
			session.Notes,
		)
		if err != nil {
			return fmt.Errorf("import workout session: %w", err)
		}
		sessionID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("read imported session id: %w", err)
		}

		for i, set := range session.Sets {
			setNumber := set.SetNumber
			if setNumber <= 0 {
				setNumber = i + 1
			}
			completed := 0
			if set.Completed {
				completed = 1
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO exercise_sets (session_id, exercise_name, set_number, target_reps, actual_reps, weight, completed)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				sessionID,
				strings.TrimSpace(set.ExerciseName),
				setNumber,
				set.TargetReps,
				set.ActualReps,
				set.Weight,
				completed,
			); err != nil {
				return fmt.Errorf("import exercise set: %w", err)
			}
		}
	}

	for _, w := range backup.StandaloneWorkouts {
		createdAt := w.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO standalone_workouts (user_id, title, created_at, notes)
			 VALUES (?, ?, ?, ?)`,
			userID, strings.TrimSpace(w.Title), createdAt, w.Notes,
		)
		if err != nil {
			return fmt.Errorf("import standalone workout: %w", err)
		}
		workoutID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("read imported standalone workout id: %w", err)
		}

		for idx, item := range w.Items {
			position := item.Position
			if position <= 0 {
				position = idx + 1
			}
			res, err := tx.ExecContext(ctx,
				`INSERT INTO standalone_workout_items (
					workout_id, position, exercise_name, exercise_type, sets, target_reps, weight, time_minutes, distance_miles
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				workoutID,
				position,
				strings.TrimSpace(item.ExerciseName),
				strings.TrimSpace(item.ExerciseType),
				item.Sets,
				item.TargetReps,
				item.Weight,
				item.TimeMinutes,
				item.DistanceMiles,
			)
			if err != nil {
				return fmt.Errorf("import standalone workout item: %w", err)
			}

			if len(item.SetScheme) == 0 {
				continue
			}
			itemID, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("read imported standalone workout item id: %w", err)
			}
			for schemeIdx, set := range item.SetScheme {
				position := set.Position
				if position <= 0 {
					position = schemeIdx + 1
				}
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO standalone_workout_item_sets (workout_item_id, position, target_reps, weight)
					 VALUES (?, ?, ?, ?)`,
					itemID,
					position,
					set.TargetReps,
					set.Weight,
				); err != nil {
					return fmt.Errorf("import standalone workout item set: %w", err)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit backup import: %w", err)
	}
	return nil
}

// SessionOwner returns the user_id that owns a session (to prevent cross-user access)
func (s *ProgressionService) SessionOwner(ctx context.Context, sessionID int64) (int, error) {
	var userID int
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM workout_sessions WHERE id = ?`, sessionID,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("session %d not found", sessionID)
	}
	return userID, err
}

type ProgressPoint struct {
	Date  string
	Value float64
}

type CardioProgressPoint struct {
	Date         string
	DurationMins float64
	Distance     float64
}

type SessionSummary struct {
	ID          int64
	WorkoutName string
	Finished    bool
	CreatedAt   time.Time
}

func (s *ProgressionService) ListSessionHistory(ctx context.Context, userID int, limit int) ([]SessionSummary, error) {
	return s.ListSessionHistoryPage(ctx, userID, limit, 0)
}

func (s *ProgressionService) ListSessionHistoryPage(ctx context.Context, userID int, limit int, offset int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workout_name, finished_at IS NOT NULL, created_at
		 FROM workout_sessions
		 WHERE user_id = ?
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list session history: %w", err)
	}
	defer rows.Close()

	out := make([]SessionSummary, 0)
	for rows.Next() {
		var ssum SessionSummary
		if err := rows.Scan(&ssum.ID, &ssum.WorkoutName, &ssum.Finished, &ssum.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session history: %w", err)
		}
		out = append(out, ssum)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ProgressionService) CountSessions(ctx context.Context, userID int) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1)
		 FROM workout_sessions
		 WHERE user_id = ?`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return count, nil
}

func (s *ProgressionService) CountOpenSessions(ctx context.Context, userID int) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1)
		 FROM workout_sessions
		 WHERE user_id = ? AND finished_at IS NULL`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count open sessions: %w", err)
	}
	return count, nil
}

func (s *ProgressionService) FinishOpenSessions(ctx context.Context, userID int) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id
		 FROM workout_sessions
		 WHERE user_id = ? AND finished_at IS NULL
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("query open sessions: %w", err)
	}
	defer rows.Close()

	sessionIDs := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scan open session id: %w", err)
		}
		sessionIDs = append(sessionIDs, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate open sessions: %w", err)
	}

	finished := 0
	for _, sessionID := range sessionIDs {
		if _, err := s.FinishSession(ctx, sessionID, userID); err != nil {
			return finished, fmt.Errorf("finish open session %d: %w", sessionID, err)
		}
		finished++
	}

	return finished, nil
}

func (s *ProgressionService) StrengthProgressSeries(ctx context.Context, userID int) (map[string][]ProgressPoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT es.exercise_name, DATE(ws.finished_at), MAX(es.weight)
		 FROM workout_sessions ws
		 JOIN exercise_sets es ON es.session_id = ws.id
		 WHERE ws.user_id = ? AND ws.finished_at IS NOT NULL AND COALESCE(es.completed, 0) = 1
		   AND es.exercise_name IN ('Squat', 'Bench Press', 'Barbell Row', 'OHP', 'Deadlift')
		 GROUP BY es.exercise_name, DATE(ws.finished_at)
		 ORDER BY DATE(ws.finished_at) ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query progress series: %w", err)
	}
	defer rows.Close()

	series := map[string][]ProgressPoint{}
	for rows.Next() {
		var exercise string
		var day string
		var value float64
		if err := rows.Scan(&exercise, &day, &value); err != nil {
			return nil, fmt.Errorf("scan progress point: %w", err)
		}
		series[exercise] = append(series[exercise], ProgressPoint{Date: day, Value: value})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return series, nil
}

func (s *ProgressionService) CardioProgressSeries(ctx context.Context, userID int) (map[string][]CardioProgressPoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT es.exercise_name, DATE(ws.finished_at), MAX(es.target_reps), MAX(es.weight)
		 FROM workout_sessions ws
		 JOIN exercise_sets es ON es.session_id = ws.id
		 WHERE ws.user_id = ? AND ws.finished_at IS NOT NULL AND COALESCE(es.completed, 0) = 1
		   AND es.exercise_name IN ('Treadmill', 'Exercise Bike', 'Staircase', 'Elliptical')
		 GROUP BY es.exercise_name, DATE(ws.finished_at)
		 ORDER BY DATE(ws.finished_at) ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query cardio progress series: %w", err)
	}
	defer rows.Close()

	series := map[string][]CardioProgressPoint{}
	for rows.Next() {
		var exercise string
		var day string
		var duration float64
		var distance float64
		if err := rows.Scan(&exercise, &day, &duration, &distance); err != nil {
			return nil, fmt.Errorf("scan cardio progress point: %w", err)
		}
		if exercise == "Staircase" || exercise == "Elliptical" {
			distance = 0
		}
		series[exercise] = append(series[exercise], CardioProgressPoint{Date: day, DurationMins: duration, Distance: distance})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return series, nil
}
