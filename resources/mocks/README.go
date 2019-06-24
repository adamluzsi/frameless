// Package mocks provide a pregenerated gomock file for working with tests.
// if you interested in testing your component more like with implementation that behave closely to a real storage,
// you may find it interesting to check out the memorystorage package.
// The primary goal for this pkg to test Rainy paths in your interactors,
// which is more complicated to properly set up using real implementations.
//
package mocks

//go:generate mockgen -package mocks -destination MockResource.go github.com/adamluzsi/frameless/resources/specs Resource
//
