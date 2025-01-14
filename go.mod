module foubot2

go 1.13

require (
	// Stay on gocal v0.9.0. It seems v0.9.1 bumped up the required 'go' version
	// to 1.21 (unclear why), which would force us to also require 1.21, but we
	// want to continue building on older Go (eg. on Debian 12 with 1.19).
	github.com/apognu/gocal v0.9.0
	github.com/jonboulle/clockwork v0.2.2
	github.com/mattermost/mattermost-server/v5 v5.39.3
	github.com/stianeikeland/go-rpio/v4 v4.6.0
	github.com/thoj/go-ircevent v0.0.0-20210723090443-73e444401d64
)
