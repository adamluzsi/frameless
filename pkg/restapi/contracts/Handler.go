package restapicontracts

// TODO:
//  - https://www.vinaysahni.com/best-practices-for-a-pragmatic-restful-api

import (
	"context"
	"net/http"
	"testing"

	"github.com/adamluzsi/testcase"
)

type Server[DTO, ID any] struct {
	MakeSubject func(testing.TB) ServerSubject
	MakeContext func(testing.TB) context.Context
}

type ServerSubject struct {
	Client       http.Client
	ResourcePath string
}

func (c Server[DTO, ID]) Name() string           { return "restapi Server" }
func (c Server[DTO, ID]) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c Server[DTO, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

func (c Server[DTO, ID]) subject() testcase.Var[ServerSubject] {
	return testcase.Var[ServerSubject]{
		ID: "restapi Server Client",
		Init: func(t *testcase.T) ServerSubject {
			return c.MakeSubject(t)
		},
	}
}

func (c Server[DTO, ID]) Spec(s *testcase.Spec) {
	s.Context("Uniform interface", c.specUniformInterface)
}

// specUniformInterface
//
// As the constraint name itself applies,
// you MUST decide APIs interface for resources inside the system which are exposed to API consumers and follow religiously.
// A resource in the system should have only one logical URI, and that should provide a way to fetch related or additional data.
// Any single resource should not be too large and contain each and everything in its representation.
// Whenever relevant, a resource should contain links (HATEOAS) pointing to relative URIs to fetch related information.
// Also, the resource representations across the system should follow specific guidelines such as naming conventions,
// link formats, or data format (XML or/and JSON).
// All resources should be accessible through a common approach such as HTTP GET and similarly modified using a consistent approach.
// Once a developer becomes familiar with one of your APIs, he should be able to follow a similar approach for other APIs.
func (c Server[DTO, ID]) specUniformInterface(s *testcase.Spec) {
	s.Describe("Create", func(s *testcase.Spec) {

	})
	s.Describe("Delete", func(s *testcase.Spec) {

	})
	s.Describe("Index", func(s *testcase.Spec) {

	})

}

// specClientServer
//
// This constraint essentially means that client applications
// and server applications MUST be able to evolve separately without any dependency on each other.
// A client should know only resource URIs, and that’s all. Today, this is standard practice in web development,
// so nothing fancy is required from your side. Keep it simple.
// Servers and clients may also be replaced and developed independently, as long as the interface between them is not altered.
func (c Server[DTO, ID]) specClientServer(s *testcase.Spec) {

}

// specStateless
//
// Roy fielding got inspiration from HTTP, so it reflects in this constraint.
// Make all client-server interactions stateless.
// The server will not store anything about the latest HTTP request the client made.
// It will treat every request as new. No session, no history.
//
// If the client application needs to be a stateful application for the end-user,
// where the user logs in once and does other authorized operations after that,
// then each request from the client should contain all the information necessary to service the request.
// including authentication and authorization details.
//
// No client context shall be stored on the server between requests.
// The client is responsible for managing the state of the application.
func (c Server[DTO, ID]) specStateless(s *testcase.Spec) {

}
