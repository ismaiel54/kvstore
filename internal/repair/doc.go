// Package repair provides conflict reconciliation logic for resolving
// concurrent versions using vector clocks. It computes the maximal set
// of winning versions and identifies stale replicas for read repair.
package repair
