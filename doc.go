// Package harness provides a small, embeddable agent loop for Go.
//
// A Harness connects a Model to ordinary Go functions. The model may answer
// directly or request one or more tools; Harness executes those tools, appends
// their results to the conversation, and calls the model again until it
// produces a final answer.
//
// The core package intentionally has no third-party dependencies. Model
// providers and application-specific tools can be implemented outside it.
package harness
