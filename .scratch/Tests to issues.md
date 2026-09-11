
- [x] Container-Quadlets are named as systemd-cotainername
	But this rule does not apply when a name is defined in Quadletfile. 
	Test both cases! Logs endpoint
- [x] list quadlets with filter
- [ ] pod managment
- [x] split api-dok in seperate files
- [ ] containers assigned to a pod get only their one logs. Not those from the pod
- [ ] integration test machines endpoints
- [x] multi user test
	- [x] can not access machines where the user has no access
- [ ] embed frontend to single go app
- [ ] (check for) update single quadlet
- [ ] rename machine to host

---

- [ ] volumes managment
	- [ ] populate volume with files
- [x] secrets managment?
- [ ] pod managment
- [x] machine online check

- [ ] Timeouts for calls to machines (backed in podman?) 
- [ ] DB Index (multiple) for performance
- [ ] logging with one logging lib
- [ ] encrypt ssh-keys with login credentials somehow? 
- [ ] remote docker system prune
- [ ] User managment
	- [ ] change own password
- [ ] copy quadlet/container to other machine
	- [ ] also copy volumes
- [ ] save/load machine state includeing
	- [ ] images
	- [ ] quadlets
	- [ ] volumes
	- [ ] secrets?
	- [ ] running container
- [ ] homepage plugin
	- [ ] stats page (or endpoint only)


Review notes (not acted on, pre-existing/out of scope):  RateLimiter.Cleanup()  is still never started (both limiters'
maps grow unboundedly), and CONTEXT.md's rate-limit section predates the per-IP scope.

frontend:
~~• The mutation-as-Promise bridge is duplicated across ~6 components; worth extracting a shared helper later.~~
•  input as CreateMachineInput  cast at MachinesPage.tsx:58 — the  MachineForm  callback type could be tightened.


Add machine groups. Everyone with write access to a machine can assign and unassign it to groups.
One machine can have multple groups.
All operations for a single machine can also bbe done on a group

Note: review found one pre-existing flake unrelated to this slice —
TestQuadletHandler_GetLogs_StreamingClientDisconnect  has a data race under  -race  that also fails on the unmodified tree; not fixed here.


---

findings durch usage:
- quadlets page
	- quadlets check for updates 
		- remove "DRY RUN" badge
		- remove log
		- tell which version is installed and to which version can be updated (even possible?) 
		- add update button
	- quadlets update all 
		- instead of "Running..." next to the spinner tell what container is been pulled/updated
		- no need to show the plain result response at the end

- remove Dashboard (What should there be?) 
- Machines Overview
	- ~~Go to machine when click an card (delte View button)~~
	- Hover effekt
- Machines Settings
	- "Grant access" Button is to small (text does not fit in button) 
	- "User id" should be a dropdown menue 
- After doing nothing for a while: error message "TOKEN_EXPIRED Token has expired"
- ~~redirect to main page on 404 link~~
- container page
	- list open ports alongside container / running network=host
- ~~reloading on a page (e.g. quadlets page) leads to page not found~~
- when creating container 
	- and backend gives  "no such image" -> frontend only shows "INTERNAL_ERROR, An unexpected error occurred"; Give better error message
	- image is not automatically pulled if not present; Give a checkbox to pull if not there
- states of container/quadlets are not live


~~Problem: quadlets page~~
	~~- lifecycle buttons~~
		~~- only show start when state is not running~~
		~~- only show stop when state is running~~
~~Promt:~~
~~/implement frontend, on the quadlets page there are buttons to control the lifecycle of each quadlet.~~
~~only show start button when state of quadlet is not running.~~
~~only show stop  button when state of quadlet is running.~~

~~Problem: logs not in a window -> too small!~~
~~Promt:~~
~~/implement on the frontend when viewing container or quadlet logs, a small window opens and shows the logs. But the window is too small for comfortable reading. Instead open the logs on a new page.~~

~~Problem: images page - delete images  currently not in use~~
~~Promt:~~
~~/to-tickets on the images page should be a button to delte all unused images. Add an endpoint for that in the backend. See in Podman repository for the call to podman at pkg/domain/tunnel/images.go ,Line 101-113~~


Problem: container page delete not running container
Promt:
/to-tickets on the conatiner page should be a button to delte all not running container. Add an endpoint for that in the backend. See in Podman repository for the call to podman at pkg/domain/tunnel/container.go ,Line 320-323