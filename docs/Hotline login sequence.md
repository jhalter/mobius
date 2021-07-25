## Hotline v.1.2.3 Login Sequence

### TLDR

1. TCP handshake
2. Hotline proto handshake
3. Client fires off:

	a. Login transaction (107)

	b. tranGetUserNameList (300)

	c. tranGetMsg (101)
	
4. Server replies to each 

### Long version

1. Client -> Server TCP handshake
2. Hotline proto handshake (TRTPHOTL)
3. Client sends Login transaction (107)

	```
	00.         // flags
	00          // isReply
	00 6b       // transaction type: Login transaction (107)
	00 00 00 01 // ID
	00 00 00 00 // error code
	00 00 00 13 // total size 
	00 00 00 13 // data size
	
	00 02 // field count
	
	00 66 // fieldUserName (102)
	00 07 // field length
	75 6e 6e 61 6d 65 64 // unnamed
	
	00 68  // fieldUserIconID (104)
	00 02  // field length
	07 d1  // 1233
	```

4. Server sends empty reply to login transaction (107)

		00 
		01 
		00 00 
		00 00 00 01
		00 00 00 00
		00 00 00 02 
		00 00 00 02 
		00 00


5. Client sends tranGetUserNameList (300) (with some weird extra data at the end??)


		00
		00 
		01 2c        // tranGetUserNameList (300)
		00 00 00 02  // ID
		00 00 00 00  // Error Code
		00 00 00 02 
		00 00 00 02 
		00 00 
		00 00
		00 65 00 00 00 03 00 00 00 00 00 00 
		
		00 02 
		00 00
		
		00 02
		00 00                                    


Server sends tranServerMsg

		00
		00 
		00 68 			 // tranServerMsg (104)
		00 00 00 01
		00 00 00 00
		00 00 00 a4
		00 00 00 a4
		
		00 01 
		
		00 65 		// fieldData (101)
		00 9e
		54 68 65 20 73 65 72 76 65 72 20 79 6f 75
		20 61 72 65 20 63 6f 6e 6e 65 63 74 65 64 20 74
		6f 20 69 73 20 6e 6f 74 20 6c 69 63 65 6e 73 65
		64 20 61 6e 64 20 69 73 20 66 6f 72 20 65 76 61
		6c 75 61 74 69 6f 6e 20 75 73 65 20 6f 6e 6c 79
		2e 20 50 6c 65 61 73 65 20 65 6e 63 6f 75 72 61
		67 65 20 74 68 65 20 61 64 6d 69 6e 69 73 74 72
		61 74 6f 72 20 6f 66 20 74 68 69 73 20 73 65 72
		76 65 72 20 74 6f 20 70 75 72 63 68 61 73 65 20
		61 20 6c 69 63 65 6e 73 65 64 20 63 6f 70 79 2e


## Hotline 1.9.2 Login Sequence


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
