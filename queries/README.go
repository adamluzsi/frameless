/*
	Package queries



	Summary

	This package implements generic CRUD definitions that commonly appears upon interacting with external resources.



	Minimum Requirement

	In order to make this package work, you have to implement the SaveEntity, FindByID and Purge query.
	Most of the other queries depends either on above mentioned queries in different combination.
	The Purge query mainly necessary for setting up test cases.
	By implementing the Purge query, you have to keep in mind,
	that your database during testing will be purged for cleanup purpose,
	and if you need something to be present in your other tests,
	you have to ensure that data by explicit context creation for the given test that depends on that.
	Also I personally highly suggest to clean up after each of your test run, that modify the external resource state.
	One last thing this package queries depends on is the presence of ID field in the received data structures / entities.

 */
package queries
