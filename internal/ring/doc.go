// Package ring implements a consistent hashing ring with virtual nodes.
// It maps keys to physical nodes while minimizing key movement when
// membership changes and supports selection of replica preference lists.
package ring
