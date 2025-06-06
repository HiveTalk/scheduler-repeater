# Vanilla 30312

This golang script will poll the vanilla hivetalk api and see the presence of a room. If there is a room with a nostr pubkey as the moderator, then it will broadcast it to the nostr relay of choice as a 30312 event with the appropriate data on open and again on close. 

## Example Hivetalk Vanilla API data: 

```sh
export BASE_URL = 'https://hivetalk.org'

curl -X 'GET' \
  '${BASE_URL}/api/v1/meetings' \
  -H 'accept: application/json' \
  -H 'authorization: ${HIVETALK_API_KEY}'
```

Response: 

```
{
  "meetings": [
    {
      "roomId": "56377RedLizard",
      "peers": [
        {
          "name": "giraffe",
          "presenter": true,
          "npub": "npub128jtgey22jdx90f7vecpy2unrn4usu3mcrlhaqpjlcy8kq8t8k7sldgax3",
          "pubkey": "51e4b4648a549a62bd3e6670122b931cebc8723bc0ff7e8032fe087b00eb3dbd",
          "lnaddress": null
        },
        {
          "name": "Bikatili",
          "presenter": false,
          "npub": null,
          "pubkey": null,
          "lnaddress": null
        }
      ]
    }
  ]
}
```

Response headers

```
 access-control-allow-origin: *  
 connection: keep-alive  
 content-length: 335  
 content-type: application/json; charset=utf-8  
 date: Thu,27 Feb 2025 22:18:42 GMT  
 etag: W/"14f-EYHkDnrnEQwYXofLhrON7j+7lu0"  
 server: nginx/1.18.0 (Ubuntu)  
 vary: Accept-Encoding  x-powered-by: Express 
```


## Example 30312 data

The above Hivetalk Vanilla API data should be reformatted and sent to the Relay as 30312 event data in this format:

- The "presenter" in Hivetalk Vanilla API data is now labeled as "Owner". 
- The room is the roomId from Hivetalk Vanilla API data

- The pubkey is specified in an environment variable.
- The sig is the signature by the pubkey. See example code for signing the event. 

- The 'd' identifier is a randomly generated unique identifier that is tied to the "room". We save this key/pair value in a simple lookup table that persists in either a simple database or a flat file. See sample code for generating the 'd' value.

- The 'status' field is either "open" or "closed", depending on if the room is open with participants or if its closed and no longer has activity. If the Hivetalk Vanilla API data no longer has the room listed and it was listed in the previous polling minute, then it means the room is now closed.

- The following values are set in the environment variables: summary, image, relays, any number of 't' tags.

```
[
	{ 
        "content":"",
        "created_at":1740218511,
        "id":"216d9d33b1f0013144c886eea66f6e590811f69a99f6b65037d5bac6bebac7a6",
        "kind":30312,
	"pubkey":"3878d95db7b854c3a0d3b2d6b7bf9bf28b36162be64326f5521ba71cf3b45a69",
	"sig":"bc9ebd8639121094b9218416f63bcccaae6c685bea434f0086f364f2b4189836692366903feb6fe373eba450efdad3862ef3ceaf7fa73f231b0fdfdc55a4ac0f",
        "tags":[
            ["d","eeueo1lua5"],
            ["room","56377RedLizard"],
            ["summary","dsecrlkjasdf"],
            ["status", "open"],
            ["image","https://image.nostr.build/56795451a7e9935992b6078f0ee40ea4b0013f8efdf954fb41a3a6a7c33f25a7.png"],
	    ["service","https://hivetalk.org/join/56377RedLizard"],
            ["p","3878d95db7b854c3a0d3b2d6b7bf9bf28b36162be64326f5521ba71cf3b45a69","owner"],
            ["t","hivetalk"],
            ["t","interactive room"],
            ["relays","wss:/hivetalk.nostr1.com","wss://honey.nostr1.com"]
        ]
    }
]
```


## Publishing to two relays

NO spaces between relays for the RELAY_URLS

```sh
export RELAY_URLS="wss://honey.nostr1.com,wss://hivetalk.nostr1.com"
export NOSTR_PVT_KEY='private-key-for-nostr-bot'
```

Dependency:

```bash
go get github.com/nbd-wtf/go-nostr
```

Example code:

```go

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

func main() {
	// Get private key from environment variable
	sk := os.Getenv("NOSTR_PVT_KEY")
	if sk == "" {
		fmt.Println("Error: NOSTR_PVT_KEY environment variable is not set")
		return
	}

	// Get public key from private key
	pub, err := nostr.GetPublicKey(sk)
	if err != nil {
		fmt.Printf("Error getting public key: %v\n", err)
		return
	}

	// Create event
	ev := nostr.Event{
		PubKey:    pub,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindTextNote,
		Tags:      nil,
		Content:   "Hello World!",
	}

	// Sign the event
	if err := ev.Sign(sk); err != nil {
		fmt.Printf("Error signing event: %v\n", err)
		return
	}

	// Get relay URLs from environment variable
	relayURLs := os.Getenv("RELAY_URLS")
	if relayURLs == "" {
		fmt.Println("Error: RELAY_URLS environment variable is not set")
		return
	}

	// Split relay URLs (assuming they're comma-separated)
	urls := strings.Split(relayURLs, ",")

	// Publish to each relay
	ctx := context.Background()
	for _, url := range urls {
		// Trim any whitespace and remove quotes
		url = strings.Trim(strings.TrimSpace(url), "'\"")

		relay, err := nostr.RelayConnect(ctx, url)
		if err != nil {
			fmt.Printf("Error connecting to relay %s: %v\n", url, err)
			continue
		}
		defer relay.Close()

		if err := relay.Publish(ctx, ev); err != nil {
			fmt.Printf("Error publishing to %s: %v\n", url, err)
			continue
		}
		fmt.Printf("Published to %s\n", url)
	}
}

```
