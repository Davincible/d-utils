package debug

import (
	"bytes"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"
	"unsafe"

	_ "net/http/pprof"
)

func init() {
	SetupGoroutineDumper(30*time.Second, FilterConfig{
		// Only show goroutines containing these patterns
		IncludePatterns: []string{
			"github.com/MemePadSol",
		},
	})
}

func formatSize(size uint64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := uint64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func addCommas(n uint64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}
	return addCommas(uint64(len(str)-3)) + "," + str[len(str)-3:]
}

func getSize(v reflect.Value, seen map[uintptr]bool) uint64 {
	if !v.IsValid() {
		return 0
	}

	// Handle all pointer types
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return uint64(unsafe.Sizeof(v.Interface()))
		}
		if seen[v.Pointer()] {
			return 0 // Already counted this pointer
		}
		seen[v.Pointer()] = true
		return uint64(unsafe.Sizeof(v.Interface())) + getSize(v.Elem(), seen)
	}

	size := uint64(v.Type().Size())

	switch v.Kind() {
	case reflect.Slice:
		if v.Len() > 0 {
			elemSize := uint64(v.Type().Elem().Size())
			size += uint64(v.Len()) * elemSize
			// If it's a slice of pointers, resolve each pointer
			if v.Type().Elem().Kind() == reflect.Ptr {
				for i := 0; i < v.Len(); i++ {
					size += getSize(v.Index(i), seen)
				}
			}
		}
	case reflect.String:
		size += uint64(v.Len())
	case reflect.Map:
		if v.Len() > 0 {
			for _, key := range v.MapKeys() {
				size += getSize(key, seen)
				size += getSize(v.MapIndex(key), seen)
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			size += getSize(v.Field(i), seen)
		}
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			size += getSize(v.Index(i), seen)
		}
	case reflect.Interface:
		if !v.IsNil() {
			size += getSize(v.Elem(), seen)
		}
	}

	return size
}

func PrintSize(name string, v interface{}) {
	t := reflect.TypeOf(v)
	rv := reflect.ValueOf(v)
	seen := make(map[uintptr]bool)
	size := getSize(rv, seen)

	fmt.Printf("%s: %s (%s)\n", name, formatSize(size), addCommas(size))

	if t.Kind() == reflect.Struct {
		fmt.Println("  Struct fields:")
		var totalSize uint64
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldSize := getSize(rv.Field(i), make(map[uintptr]bool))
			totalSize += fieldSize
			fmt.Printf("    %s: %s (%s)\n", field.Name, formatSize(fieldSize), addCommas(fieldSize))
		}
		fmt.Printf("  Total struct size: %s (%s)\n", formatSize(totalSize), addCommas(totalSize))
	}
}

// FilterConfig defines what to include/exclude from the goroutine dump
type FilterConfig struct {
	// Exclude goroutines whose stack contains any of these strings
	ExcludePatterns []string
	// Only include goroutines whose stack contains any of these strings
	IncludePatterns []string
	// Minimum duration for goroutine to be included (for "sleep" states)
	MinDuration time.Duration
	// Only show goroutines in these states (running, runnable, sleep, etc)
	States []string
}

// DumpGoroutineStack prints filtered stack traces of current goroutines
func DumpGoroutineStack(config FilterConfig) {
	buf := make([]byte, 1024*1024)
	n := runtime.Stack(buf, true)

	// Split the stack dump into individual goroutine sections
	goroutines := bytes.Split(buf[:n], []byte("\n\n"))

	fmt.Println("\n=== Filtered Goroutine dump ===")

	for _, g := range goroutines {
		stack := string(g)

		// Skip empty stacks
		if len(stack) == 0 {
			continue
		}

		// Apply filters
		if shouldIncludeGoroutine(stack, config) {
			fmt.Printf("%s\n\n-------------------------------------------------------------------------------\n", stack)
		}
	}

	fmt.Println("=== End of Goroutine dump ===\n\n")
}

func shouldIncludeGoroutine(stack string, config FilterConfig) bool {
	// Check exclude patterns
	for _, pattern := range config.ExcludePatterns {
		if strings.Contains(stack, pattern) {
			return false
		}
	}

	// Check include patterns (if any are specified)
	if len(config.IncludePatterns) > 0 {
		matched := false
		for _, pattern := range config.IncludePatterns {
			if strings.Contains(stack, pattern) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check state filters
	if len(config.States) > 0 {
		stateMatched := false
		for _, state := range config.States {
			if strings.Contains(stack, fmt.Sprintf("[%s", state)) {
				stateMatched = true
				break
			}
		}
		if !stateMatched {
			return false
		}
	}

	// Check duration for sleeping goroutines
	if config.MinDuration > 0 && strings.Contains(stack, "[sleep]") {
		// Parse duration from stack trace (this is a simplified check)
		if strings.Contains(stack, "minutes") || strings.Contains(stack, "hours") {
			return true
		}
		// Add more precise duration parsing if needed
	}

	return true
}

// SetupGoroutineDumper sets up periodic goroutine dumps with filtering
func SetupGoroutineDumper(dumpInterval time.Duration, config FilterConfig) {
	// Periodic dumps
	if dumpInterval > 0 {
		go func() {
			for {
				time.Sleep(dumpInterval)
				DumpGoroutineStack(config)
			}
		}()
	}

	// CPU profiling setup remains the same
	// f, err := os.Create("cpu.pprof")
	// if err != nil {
	// 	fmt.Printf("Could not create CPU profile: %v\n", err)
	// 	return
	// }
	// defer f.Close()
	//
	// if err := pprof.StartCPUProfile(f); err != nil {
	// 	fmt.Printf("Could not start CPU profile: %v\n", err)
	// 	return
	// }
	// defer pprof.StopCPUProfile()
}
