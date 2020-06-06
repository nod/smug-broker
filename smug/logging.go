// wrapper around our logging so we can setup certain attributes easily

package smug

import (
	"os"

	log "github.com/sirupsen/logrus"
)

type Logger struct {
	log.Entry
}

func (lg *Logger) logMetrics(rcvd int64, sent int64) {
	lg.WithFields(log.Fields{
		"rcvd": rcvd,
		"sent": sent,
	}).Info("heartbeat")
}

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.WarnLevel)
}

func SetupLogging(loglevel string) {
	if lev, err := log.ParseLevel(loglevel); err == nil {
		log.SetLevel(lev)
	} else {
		log.Panic("invalid loglevel")
	}
}

func NewLogger(key string, context string) *Logger {
	return &Logger{*log.WithFields(log.Fields{key: context})}
}
