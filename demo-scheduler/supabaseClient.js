import dotenv from 'dotenv';
import { createClient } from '@supabase/supabase-js';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Load env from parent directory
dotenv.config({ path: path.join(__dirname, '..', '.env') });

const config = {
  supabase_url: process.env.SUPABASE_URL,
  supabase_key: process.env.SUPABASE_ANON_KEY
}

console.log('Supabase config.... in supabaseClient.js', config)

export const supabase = createClient(
  config.supabase_url,
  config.supabase_key,
);
