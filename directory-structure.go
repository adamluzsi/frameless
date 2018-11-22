/*

Screaming Architecture

A human oriented directory structure can decrease the project learning curve for new team members.
I had the pleasure to work with people I highly respect because of they personality and/or experience in software engineering.
Usually each team has it's own flavor when it comes to structuring the project code base.
I have to credit, most of the reasoning really easily fits into a developer thinking when they are in the "flow",
so there is always a point of view that able to justify why something should be organized in one way or another.
Most of the approach that I learned from different sources has usually at least one thing in common.
They organize code base usually around code dependencies or around class hierarchy.
I feel that this by on its own not a good or bad thing at all.

The sneaky problems usually unfold when a new team member join the project,
and he needs someone actively to understand the high level concepts what this project try to achieve.
If the domain knowledge implicitly required for the project to be understood,
it will be harder for the new recruit to join to productive work.

My directory structuring was no exception to this, until I received a really important feedback.
It came from my wife. She often sit next to me when I sometimes work at home, and she mentioned,
that she likes my code because she can read it like a book.
This gave me the idea for the directory structuring then.
My wife is not in the IT, yet I love to ask her opinion on my creations,
because she always give back a honest opinion on the subjects.
And one of the most important lesson I learned by asking her opinion,
is that the more dependency and IT knowledge required for a project,
the more time it takes to present her and explain her what I currently work on, which usually correlate with new recruits learning curve.
So I started to keep in mind that my application should be easy to understood on high level, even without programming experience.
Therefore I usually try my best to split codebase trough domain parts.
Than I put those domain parts into separate directories, called suffixed with "services".
A service here in this terminology means purely domain functionality and nothing regarding External Interfaces like HTTP API.

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
