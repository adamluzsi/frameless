/*

Screaming Architecture

A human oriented directory structure can decrease the project learning curve for new team members.
Usually each team has it's own flavor when it comes to structuring the project code base.
Some framework may even prefer a folder structure idiom based on MVC or similar architectural pattern.

When a new member joins the team, usually this is the first layer that person encounter.
And if they need someone else who actively help them to understand the high level concepts of that project,
then usually there is a good change to find some task regarding improvements in the project structuring.
If the domain knowledge implicitly required for the project to be understood,
it will be harder for the new recruit to join to productive work.

The directory structuring I prefer aims to express project audience and they use-cases which together hopefully describe the project purpose.
In the subject, the feedback that helped me the most came from my wife.
She often sit next to me when I work at home, and she mentioned,
that she likes my code because she can read it like a book.
This gave me the idea for the directory structuring then.
My wife is not an IT specialist, therefore her feedback especially valuable in this topic.
It is because the more dependency and IT knowledge required for a project to be undershoot clearly on high level,
usually lineal with the project learning curve of the new team members.
So I started to keep in mind that my application should be easy to understood on high level, even without programming experience.
Therefore I usually try my best to split codebase trough domain parts like audience, use-case and product line.
Product lines usually represented as top level directories suffixed with "-service".
Services in this terminology not necessarily mean an external interface like HTTP API,
but a set of use-case that connected together.

Let me present it with an example.
This is an application I create for my friend as a present.
I don't tell you the Application functionality, because that would ruin the experiment here.
I on purpose excluded the _test.go files, the vendor directory, and the ext resource and ext interfaces, to remove noise.
In theory you should be able to find out more or less what this application do,
what type of audience has the SRP here nd what features the application provides.
The testing packages in each service directory is a common entry-point for specification management towards external resources,
That will be explained in a separate section.

	.
	├── Doctor.go
	├── Patient.go
	├── appointment-service
	│   ├── Appointment.go
	│   ├── assistants
	│   │   └── AssignAppointmentToDoctor.go
	│   ├── doctors
	│   │   ├── AcceptAppointmentRequest.go
	│   │   ├── DefineTreatmentPlan.go
	│   │   ├── ListAppointments.go
	│   │   └── SyncWithCloudCalendar.go
	│   ├── patients
	│   │   ├── ListAppointment.go
	│   │   ├── ReceiveAppointmentBookingFromDoctor.go
	│   │   └── RequestAppointment.go
	│   └── testing
	│       └── Test.go
	├── catalog-service
	│   ├── assistants
	│   │   ├── ListPatientByCriterion.go
	│   │   ├── UpdateDiseaseJournal.go
	│   │   └── ViewDiseaseJournal.go
	│   ├── patients
	│   │   ├── CreateNoteAboutAllergen.go
	│   │   ├── ListAllergen.go
	│   │   └── SeeDiseaseJournal.go
	│   └── testing
	│       └── Test.go
	└── warehouse-service
		└── managers
			├── RegisterConsume.go
			└── RegisterNewConsumables.go


If you was not able to find out by now, please send me a mail or leave open an issue about this,
and tell me what was your experience with this.
There is no wrong answer, so don't hesitate to express your opinion and experience.

*/
package frameless
