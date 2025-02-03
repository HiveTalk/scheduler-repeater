import { supabase } from './supabaseClient.js';
import { sendNewEvent } from './utils.js';

const CURRENT_TIME = new Date();
const TWO_MINUTES_MS = 2 * 60 * 1000; // 2 minutes in milliseconds

async function fetchUpcomingEvents() {
    const timeMin = new Date(CURRENT_TIME.getTime() - TWO_MINUTES_MS).toISOString();
    const timeMax = new Date(CURRENT_TIME.getTime() + TWO_MINUTES_MS).toISOString();

    // Fetch events that are starting
    const { data: startingEvents, error: startError } = await supabase
        .from('events')
        .select('*')
        .gte('start_time', timeMin)
        .lte('start_time', timeMax)
        .not('status', 'eq', 'live:sent');

    if (startError) {
        console.error('Error fetching starting events:', startError);
        return [];
    }

    // Fetch events that are ending
    const { data: endingEvents, error: endError } = await supabase
        .from('events')
        .select('*')
        .gte('end_time', timeMin)
        .lte('end_time', timeMax)
        .not('status', 'eq', 'ended:sent');

    if (endError) {
        console.error('Error fetching ending events:', endError);
        return [];
    }

    // Process starting events
    const startingPromises = (startingEvents || []).map(event =>
        sendNewEvent(event, 'live')
            .catch(error => {
                console.error('Failed to send starting event:', event.id, error);
                return null;
            })
    );

    // Process ending events
    const endingPromises = (endingEvents || []).map(event =>
        sendNewEvent(event, 'ended')
            .catch(error => {
                console.error('Failed to send ending event:', event.id, error);
                return null;
            })
    );

    // Process all events in parallel
    await Promise.all([...startingPromises, ...endingPromises]);

    return [...(startingEvents || []), ...(endingEvents || [])];
}

// Execute the function
fetchUpcomingEvents()
    .then(events => {
        console.log('Processed events:', events);
    })
    .catch(error => {
        console.error('Error:', error);
    });
