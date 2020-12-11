package logger

import (
	log "github.com/sirupsen/logrus"
	"os"
	"runtime"
	"strings"
)

const projectPath = "hms-redfish-translation-service/"

func SetupLogging() {
	log.WithFields(log.Fields{"LogLevel": log.GetLevel()}).Info("Logging Initialized")
}

func init() {
	logLevel := os.Getenv("LOG_LEVEL")
	logLevel = strings.ToUpper(logLevel)

	projectPathLength := len(projectPath)

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		ForceColors: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			// We know both the function and the file are going to have "redfish-translation-layer/" and then something,
			// so whack off everything up to and including that and call it a day! Pretty!
			functionIndexStart := strings.LastIndex(f.Function, projectPath)
			fileIndexStart := strings.LastIndex(f.File, projectPath)

			var funcname string
			if functionIndexStart > 0 {
				funcname = f.Function[functionIndexStart + projectPathLength:]
			} else {
				funcname = f.Function
			}

			var filename string
			if fileIndexStart > 0 {
				filename = f.File[fileIndexStart + projectPathLength:]
			} else {
				filename = f.File
			}

			return funcname, filename
		},
	})
	log.SetReportCaller(true)

	switch logLevel {
	case "TRACE":
		log.SetLevel(log.TraceLevel)
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
	case "INFO":
		log.SetLevel(log.InfoLevel)
	case "WARN":
		log.SetLevel(log.WarnLevel)
	case "ERROR":
		log.SetLevel(log.ErrorLevel)
	case "FATAL":
		log.SetLevel(log.FatalLevel)
	case "PANIC":
		log.SetLevel(log.PanicLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
}
