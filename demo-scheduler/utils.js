import { SimplePool } from 'nostr-tools/pool';
import { useWebSocketImplementation } from 'nostr-tools/pool';
import { finalizeEvent } from 'nostr-tools/pure';
import { nip19 } from 'nostr-tools';
import WebSocket from 'ws';
import { supabase } from './supabaseClient.js';

useWebSocketImplementation(WebSocket);

let defaultRelays = ['wss://honey.nostr1.com'];
const hive_url = process.env.HIVETALK_URL;

/* sendNewEvent uses the other two methods in this file. */
export async function sendNewEvent(payload, status) {
    const { data: keys_data } = await supabase
      .from('room_info')
      .select('room_npub, room_nsec, room_relay_url')
      .eq('room_name', payload.room_name)
      .single();

    console.log('room data', keys_data)

    const relayurl =  keys_data?.room_relay_url
    console.log('room_relay_url : ', relayurl)
    if (relayurl && relayurl.trim() !== '') {
      defaultRelays.push(relayurl);
    }
    // TODO add event relay url in future when front end added

    //console.log('room nsec/npub: ', keys_data);
    const nsec = keys_data?.room_nsec;
    const npub = keys_data?.room_npub;
    // console.log('room pub: ', npub);

    const pk  = await nip19.decode(nsec).data;
    const pubkey = await nip19.decode(npub).data;

    let event = await updateNip53(payload, pubkey, status)

    // sign the event
    const signedEvent = await finalizeEvent(event, pk);
    console.log('Signed Event:', signedEvent);
    const event_id = signedEvent['id'];
    console.log('Event ID', event_id);

    // add event id and status is planned for new event
    // Update nostr_event_id in the events table
    console.log("Updating event in Supabase with nostr event id");
    const { data: updateData, error: updateError } = await supabase
      .from('events')
      .update({ nostr_event_id: event_id , status: status + ':sent' })
      .eq('id', payload.id)
      .select();

    if (updateError) {
      console.error("Error updating event with nostr_event_id:", updateError);
    } else {
      console.log("Successfully updated event with nostr_event_id:", updateData);
    }

    try {
      console.log('...inside sendLiveEvent');
      await sendLiveEvent(signedEvent, pubkey, defaultRelays);
    } catch (lastError) {
      // TODO Check relay and see if msg was never send
      // may need a separate process?
      console.error('An error occurred in sendLiveEvent:', lastError);
      const { data: updateData, error: updateError } = await supabase
      .from('events')
      .update({ nostr_event_id: event_id , status: status + ':failed' })
      .eq('id', payload.id)
      .select();

      if (updateError) {
        console.error("Error updating event with nostr_event_id:", updateError);
      } else {
        console.log("Successfully updated event with nostr_event_id:", updateData);
      }
    }
}

/* format the event data from db for nostr relays */
export async function updateNip53(eventData, pubkey, status) {
  const start_time = Math.floor(new Date(eventData.start_time).getTime() / 1000);
  const end_time = Math.floor(new Date(eventData.end_time).getTime() / 1000);
  let room_name = eventData.room_name;
  // status -> planned, live, ended, deleted

  const eventParams = {
      kind: 30311,
      created_at: Math.floor(Date.now() / 1000),
      pubkey: pubkey,
      tags: [
        ['d', eventData.identifier],
        ['title', eventData.name],
        ['starts', start_time.toString()],
        ['ends', end_time.toString()],
        ['streaming', `${hive_url}/join/${room_name}`],
        ['summary', eventData.description],
        ['image', eventData.image_url],
        ['status', status],
        ['t', 'nostr'],
        ['t', 'hivetalk'],
        ['t', 'livestream'],
        ],
        content: '',
      };

  return eventParams;
}

export async function sendLiveEvent(event, publicKey, defaultRelays) {
  try {
    const eventID = event['id'];
    //const allrelays = [...hiveRelays, ...defaultRelays];
    const allrelays = defaultRelays; // default to just one relay for efficiency
    console.log('send Live Event', event);
    console.log('send Event - Relays:', allrelays);
    
    // Create pool with shorter timeout
    const pool = new SimplePool({
      getTimeout: 3000,     // 3 seconds to get events
      writeTimeout: 2000    // 2 seconds to write events
    });

    // Set up subscription asynchronously for monitoring only
    const setupSubscription = () => {
      const sub = pool.subscribeMany(
        allrelays,
        [
          {
            authors: [publicKey],
            since: Math.floor(Date.now() / 1000) - 1,
            kinds: [event.kind]
          }
        ],
        {
          onevent(e) {
            if (e.id === eventID) {
              console.log('Live Activity Event received:', e);
              sub.close();
            }
          },
          oneose() {
            sub.close();
          }
        }
      );
      
      // Auto-close subscription after 5 seconds
      setTimeout(() => sub.close(), 5000);
    };

    // Start subscription monitoring asynchronously
    setupSubscription();

    // Try publishing with retries
    const maxRetries = 2;
    let lastError;
    
    for (let i = 0; i < maxRetries; i++) {
      try {
        if (i > 0) console.log(`Retry attempt ${i + 1}/${maxRetries}`);
        await Promise.any(pool.publish(allrelays, event));
        console.log('Published successfully to at least one relay!');
        break;
      } catch (err) {
        console.log(`Failed publish attempt ${i + 1}:`, err.message);
        lastError = err;
        if (i < maxRetries - 1) {
          await new Promise(resolve => setTimeout(resolve, 500)); // Wait 500ms before retry
        }
      }
    }

    // Don't wait for verification, close pool after publishing
    pool.close(allrelays);

    if (lastError) {
      throw lastError;
    }

  } catch (error) {
    console.error('An error occurred in sendLiveEvent:', error);
    return false;
  }
  return true;
}
