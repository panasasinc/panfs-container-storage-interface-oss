// Copyright 2025 VDURA Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jsonlog

import (
	"encoding/json"
	"io"
	"os"
	"runtime/debug"
	"time"
)

// Level represents the severity level for logging.
type Level int

// Logging levels for the logger.
const (
	LevelDebug Level = iota // Debug level
	LevelInfo               // Info level
	LevelError              // Error level
	LevelFatal              // Fatal level
	LevelOff                // No logging
)

// LogField represents a set of key-value pairs for structured logging.
type LogField map[string]interface{}

// String returns the string representation of the log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return ""
	}
}

// Logger represents a structured JSON logger with different log levels.
type Logger struct {
	out      io.Writer
	minLevel Level
}

// New creates a new Logger instance.
//
// Parameters:
//
//	out      - The output destination for log messages (e.g., os.Stdout).
//	minLevel - The minimum log level to output.
//
// Returns:
//
//	*Logger - The initialized Logger instance.
func New(out io.Writer, minLevel Level) *Logger {
	return &Logger{
		out:      out,
		minLevel: minLevel,
	}
}

// PrintDebug logs a debug-level message with optional properties.
//
// Parameters:
//
//	message    - The debug message to log.
//	properties - Optional key-value pairs to include in the log.
func (l *Logger) PrintDebug(message string, properties LogField) {
	_, _ = l.print(LevelDebug, message, properties)
}

// PrintInfo logs an info-level message with optional properties.
//
// Parameters:
//
//	message    - The info message to log.
//	properties - Optional key-value pairs to include in the log.
func (l *Logger) PrintInfo(message string, properties LogField) {
	_, _ = l.print(LevelInfo, message, properties)
}

// PrintError logs an error-level message with optional properties.
//
// Parameters:
//
//	err        - The error to log.
//	properties - Optional key-value pairs to include in the log.
func (l *Logger) PrintError(err error, properties LogField) {
	_, _ = l.print(LevelError, err.Error(), properties)
}

// PrintFatal logs a fatal-level message with optional properties and exits the program.
//
// Parameters:
//
//	err        - The fatal error to log.
//	properties - Optional key-value pairs to include in the log.
//
// Exits the program with status code 1 after logging.
func (l *Logger) PrintFatal(err error, properties LogField) {
	_, _ = l.print(LevelFatal, err.Error(), properties)
	os.Exit(1)
}

// print constructs and writes a log entry if the level is above the minimum level.
//
// This is an internal method. Its return values (number of bytes written and error) are not used by public logging methods like PrintInfo and PrintError.
//
// Parameters:
//
//	level      - The log level of the message.
//	message    - The log message.
//	properties - Optional key-value pairs to include in the log.
//
// Returns:
//
//	(int, error) - Number of bytes written and any error encountered.
func (l *Logger) print(level Level, message string, properties LogField) (int, error) {
	if level < l.minLevel {
		return 0, nil
	}

	aux := struct {
		Level      string   `json:"level"`
		Time       string   `json:"time"`
		Message    string   `json:"message"`
		Properties LogField `json:"properties,omitempty"`
		Trace      string   `json:"trace,omitempty"`
	}{
		Level:      level.String(),
		Time:       time.Now().UTC().Format(time.RFC3339),
		Message:    message,
		Properties: properties,
	}

	if level >= LevelError {
		aux.Trace = string(debug.Stack())
	}

	var line []byte

	line, err := json.Marshal(aux)
	if err != nil {
		line = []byte(LevelError.String() + ": unable to marshal log message: " + err.Error())
	}

	return l.out.Write(append(line, '\n'))
}

// Write implements the io.Writer interface for the Logger.
// It logs the given message at error level.
//
// Parameters:
//
//	message - The byte slice message to log.
//
// Returns:
//
//	(int, error) - Number of bytes written and any error encountered.
func (l *Logger) Write(message []byte) (n int, err error) {
	return l.print(LevelError, string(message), nil)
}
