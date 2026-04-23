package workout

// Exercise defines a lift with its progression increments and starting weight
type Exercise struct {
	Name               string
	DefaultStartWeight float64
	IncrementBy        float64
}

// Standard SL5x5 exercises
var (
	Squat      = Exercise{Name: "Squat", DefaultStartWeight: 195.0, IncrementBy: 2.5}
	BenchPress = Exercise{Name: "Bench Press", DefaultStartWeight: 135.0, IncrementBy: 2.5}
	BarbellRow = Exercise{Name: "Barbell Row", DefaultStartWeight: 95.0, IncrementBy: 2.5}
	OHP        = Exercise{Name: "OHP", DefaultStartWeight: 95.0, IncrementBy: 2.5}
	Deadlift   = Exercise{Name: "Deadlift", DefaultStartWeight: 225.0, IncrementBy: 2.5}
)

func AllExercises() []Exercise {
	return []Exercise{Squat, BenchPress, BarbellRow, OHP, Deadlift}
}

// Program represents one of the two alternating workouts
type Program struct {
	Name      string
	Exercises []Exercise
}

var (
	WorkoutA = Program{
		Name:      "A",
		Exercises: []Exercise{Squat, BenchPress, BarbellRow},
	}
	WorkoutB = Program{
		Name:      "B",
		Exercises: []Exercise{Squat, OHP, Deadlift},
	}
)

// NextProgram returns the program that should follow the given last program name.
// If no previous workout exists, returns WorkoutA.
func NextProgram(lastProgramName string) Program {
	if lastProgramName == "A" {
		return WorkoutB
	}
	return WorkoutA
}

// ProgramByName returns a known program by its short name.
func ProgramByName(name string) (Program, bool) {
	switch name {
	case "A":
		return WorkoutA, true
	case "B":
		return WorkoutB, true
	default:
		return Program{}, false
	}
}

// SetsPerExercise is the standard SL5x5 set count
const SetsPerExercise = 5

// DeadliftSetsPerExercise is the standard StrongLifts deadlift set count.
const DeadliftSetsPerExercise = 1

// TargetReps is the standard SL5x5 rep count per set
const TargetReps = 5

func setCountForExercise(ex Exercise) int {
	if ex.Name == Deadlift.Name {
		return DeadliftSetsPerExercise
	}
	return SetsPerExercise
}
