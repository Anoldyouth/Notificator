package loggers

import "fmt"

type ConsoleLogger struct{}

func (l *ConsoleLogger) Info(message string) {
	fmt.Printf("[INFO] %s\n", message)
}

func (l *ConsoleLogger) Warning(message string) {
	fmt.Printf("[WARNING] %s\n", message)
}

func (l *ConsoleLogger) Error(message string) {
	fmt.Printf("[ERROR] %s\n", message)
}
