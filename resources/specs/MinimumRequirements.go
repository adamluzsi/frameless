package specs

import "github.com/adamluzsi/frameless/resources"

type MinimumRequirements interface {
	resources.Creator
	resources.Finder
	resources.Deleter
	resources.Truncater
}
