/*
	Package specs



	Summary

	This package implements generic CRUD operation related testing specifications that commonly appears upon interacting with external resources.



	Reason for separate pkg from resources pkg

	This package originally was part of the resources pkg,
	but package "testing" has side effects on import.
	In order to avoid force the import on the implementations,
	the specs had to be extracted into a separate package.
	Since testing#T.Run is heavily used, abstracting testing.T away is difficult.



	Minimum Requirement from Resource point of view

	In order to make this package work, you have to implement the TestMinimumRequirementsWithExampleEntities specification.
	Most of the other Resource specs specification depends on the Resource specs mentioned in the min requirement specification.
	Keep in mind, that you have no guarantee on your Resource content during test execution, because some specification
	may alter the content of the Resource (db), or delete from it.
	If you need specific data in the Resource you want to test with, you must ensure in the test execution that
	such Context is correctly provisioned, and after test execution, cleaned up.
	If you use such data-set in a external Resource that needs to be kept intact,
	I advise you to use separate environments for test execution and manual testing.



	Requirement from Business Entities

	This package depends on a fact that there is a string field ID in a business entity struct,
	or at least a tag `ext:"ID"`. This allows the package to create specifications that assumes,
	that the ID field links the EntityTypeName structure to an external Resource object.
	The Resource specs package doesn't care about the content of the ID string field,
	and don't have assumptions other than the existence of the field ID on a struct


*/
package contracts
