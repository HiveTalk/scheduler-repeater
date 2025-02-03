import { generateSecretKey, getPublicKey, nip19 } from 'nostr-tools';
import { supabase } from '../supabaseClient.js';
import dotenv from 'dotenv';

dotenv.config();

const CURRENT_TIME = new Date();

// Arrays for generating random names
const adjectives = ['Happy', 'Cosmic', 'Digital', 'Quantum', 'Crypto', 'Virtual', 'Stellar', 'Electric', 'Neon', 'Cyber'];
const nouns = ['Panda', 'Phoenix', 'Dragon', 'Unicorn', 'Tiger', 'Eagle', 'Wolf', 'Lion', 'Bear', 'Fox'];
const eventTypes = ['Meetup', 'Workshop', 'Conference', 'Hackathon', 'Summit', 'Gathering', 'Festival', 'Party', 'Session', 'Showcase'];

// Helper functions for generating random names and images
const getRandomElement = (arr) => arr[Math.floor(Math.random() * arr.length)];
const getRandomNumber = (min, max) => Math.floor(Math.random() * (max - min + 1)) + min;
const generateUsername = () => `${getRandomElement(adjectives)}${getRandomElement(nouns)}${Math.floor(Math.random() * 100)}`;
const generateRoomName = () => `${getRandomElement(adjectives)}-${getRandomElement(nouns)}-${Date.now().toString(36)}`;
const generateEventName = (index) => `${getRandomElement(adjectives)} ${getRandomElement(eventTypes)} #${index + 1}`;
const generateImage = () => {
    const width = getRandomNumber(300, 800);
    const height = getRandomNumber(300, 600);
    const randomSeed = Math.random().toString(36).substring(2, 15);
    return `https://picsum.photos/seed/${randomSeed}/${width}/${height}`;
};

// Parse command line arguments
const args = process.argv.slice(2);
let numEvents = 10; // default value
let timeOffset = null;

// Parse arguments
for (let i = 0; i < args.length; i++) {
    if (args[i] === '--in' && i + 1 < args.length) {
        const timeStr = args[i + 1];
        const minutes = parseInt(timeStr);
        if (!isNaN(minutes)) {
            timeOffset = minutes * 60 * 1000; // convert to milliseconds
        }
        i++; // skip the next argument since we used it
    } else {
        const eventsArg = parseInt(args[i]);
        if (!isNaN(eventsArg) && eventsArg > 0) {
            numEvents = eventsArg;
        } else {
            console.error('Invalid number of events specified. Using default value of 10.');
        }
    }
}

console.log(`Creating ${numEvents} test events${timeOffset ? ` starting in ${timeOffset/60000} minutes from now` : ''}...`);

async function createTestData() {
    try {
        // 1. Generate Nostr keys for new user
        const privateKey = generateSecretKey();
        const publicKey = getPublicKey(privateKey);
        const npub = nip19.npubEncode(publicKey);
        const nsec = nip19.nsecEncode(privateKey);

        // 2. Create user profile
        const username = generateUsername();
        const { data: profile, error: profileError } = await supabase
            .from('profiles')
            .insert({
                username: username,
                lightning_address: null,
                nip05: null,
                nostr_pubkey: publicKey,
                avatar_url: `https://api.dicebear.com/7.x/personas/png?seed=${encodeURIComponent(username)}`,
                website_link: 'https://example.com',
                subscriber_status: false,
                lnbits_wallet_id: null,
                preferred_relays: null
            })
            .select()
            .single();

        if (profileError) throw profileError;
        console.log('Created user profile:', profile);

        // 3. Create a room
        const roomName = generateRoomName();
        const { data: room, error: roomError } = await supabase
            .from('room_info')
            .insert({
                room_name: roomName,
                room_picture_url: generateImage(),
                room_description: `A test room created by ${profile.username}`,
                profile_id: profile.id,
                room_npub: npub,
                room_nsec: nsec,
                room_nip05: null,
                room_lightning_address: null,
                room_zap_goal: 0,
                room_visibility: true,
                save_chat_directive: false,
                room_relay_url: null
            })
            .select()
            .single();

        if (roomError) throw roomError;
        console.log('Created room:', room);

        // 4. Add events
        const events = [];
        for (let i = 0; i < numEvents; i++) {
            const eventName = generateEventName(i);

            // Calculate event time
            let startTime;
            if (timeOffset !== null) {
                startTime = new Date(CURRENT_TIME.getTime() + timeOffset);
                // If creating multiple events, space them out by 1 minute each
                if (i > 0) {
                    startTime = new Date(startTime.getTime() + (i * 60000));
                }
            } else {
                // Original random time logic
                const randomMinutes = getRandomNumber(30, 180);
                startTime = new Date(CURRENT_TIME.getTime() + randomMinutes * 60000);
            }

            const endTime = new Date(startTime.getTime() + 60 * 60000); // 1 hour duration

            const event = {
                room_name: roomName,
                naddr_id: `test-event-${Date.now()}-${i}-${Math.random().toString(36).substring(2, 15)}`,
                name: eventName,
                description: `Join us for an exciting ${getRandomElement(eventTypes).toLowerCase()} hosted by ${profile.username}!`,
                image_url: generateImage(),
                start_time: startTime.toISOString(),
                end_time: endTime.toISOString(),
                is_paid_event: false,
                ticket_price: null,
                lightning_address: null,
                event_relay: null,
                profile_id: profile.id,
                date: startTime.toISOString().split('T')[0],
                recurring: null,
                identifier: `test-${Date.now()}-${i}-${Math.random().toString(36).substring(2, 15)}`,
                nostr_event_id: null,
                status: 'planned'
            };
            events.push(event);
        }

        const { data: createdEvents, error: eventsError } = await supabase
            .from('events')
            .insert(events)
            .select();

        if (eventsError) throw eventsError;
        console.log('Created events:', createdEvents);

        console.log('Test data creation completed successfully!');
        return { profile, room, events: createdEvents };

    } catch (error) {
        console.error('Error creating test data:', error);
        throw error;
    }
}

// Run the function
createTestData()
    .then(() => process.exit(0))
    .catch((error) => {
        console.error('Fatal error:', error);
        process.exit(1);
    });
