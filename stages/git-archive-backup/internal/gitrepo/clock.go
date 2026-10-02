package gitrepo

import "time"

// now returns the current time. It is a package variable so tests can pin commit
// timestamps deterministically.
var now = time.Now
