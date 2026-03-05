package ports

type Logger interface {
	Info(message string)
	Warning(message string)
	Error(message string)
}
