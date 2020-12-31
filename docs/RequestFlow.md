1. Client/Server TCP handshake
2. Hotline Proto handshake
2. Client sends Login transaction (107)
	- 105: UserLogin
	- 106: UserPassword
	- 160: Version 
3. Server sends reply with no type and fields:
	- 160: Version
	- 161: Banner ID 
	- 162: Server Name
4. Server sends Agreement transaction (109)
5. Client sends Agreed transaction (121)
	- 102: User name
	- 104: User icon ID
	- 113: Options
	- 215: Automatic Reponse (optional)
6. Server sends reply with no type and no fields
7. Server sends User Access (354)

TBD

```

```
