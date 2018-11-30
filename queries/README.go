/*
	Package queries



	Summary

	This package implements generic CRUD definitions that commonly appears upon interacting with external resources.



	Minimum Requirement

	In order to make this package work, you have to implement the TestMinimumRequirements specification.
	Most of the other queries specification depends on the queries mentioned in the min requirement specification.
	Keep in mind, that you have no guarantee on your database content during test execution, because some specification
	may alter the content of the resource (db), or delete from it.
	If you need specific data in the resource you want to test with, you must ensure in the test execution that
	such context is correctly provisioned, and after test execution, cleaned up.
	One last thing this package queries depends on is the presence of ID field in the received data structures / entities.

*/
package queries
