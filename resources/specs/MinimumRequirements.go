package specs

import "github.com/adamluzsi/frameless/resources"

type MinimumRequirements interface {
	resources.Saver
	resources.Finder
	resources.Deleter
	resources.Truncater
}
