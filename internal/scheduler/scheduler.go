package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

type Profile string

const (
	Development  Profile = "development"
	Monitoring   Profile = "monitoring"
	DeepAnalysis Profile = "deep_analysis"
)

type Config struct {
	Interval time.Duration
	Profile  Profile
}

func Expand(suite canary.Suite, cycle int, profile Profile) canary.Suite {
	repeats := map[string]int{"easy": 10, "general": 6, "expensive": 3}
	expensiveEvery := 4
	switch profile {
	case Development:
		repeats = map[string]int{"easy": 1, "general": 1, "expensive": 1}
		expensiveEvery = 1
	case DeepAnalysis:
		expensiveEvery = 1
	case Monitoring:
	default:
		profile = Monitoring
	}
	tasks := make([]canary.Task, 0)
	for _, task := range suite.Tasks {
		count := repeats[task.Tier]
		if task.Tier == "expensive" && cycle%expensiveEvery != 0 {
			count = 0
		}
		for repeat := 0; repeat < count; repeat++ {
			copy := task
			if count > 1 {
				copy.ID = task.ID + "-repeat-" + itoa(repeat+1)
			}
			tasks = append(tasks, copy)
		}
	}
	suite.Tasks = tasks
	return suite
}

func Run(ctx context.Context, config Config, cycle func(context.Context, int) error) error {
	if config.Interval <= 0 {
		return errors.New("scheduler interval must be positive")
	}
	if cycle == nil {
		return errors.New("scheduler cycle cannot be nil")
	}
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	cycleNumber := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			cycleNumber++
			if err := cycle(ctx, cycleNumber); err != nil {
				return err
			}
		}
	}
}

func itoa(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	result := make([]byte, 0, 3)
	for value > 0 {
		result = append([]byte{digits[value%10]}, result...)
		value /= 10
	}
	return string(result)
}
