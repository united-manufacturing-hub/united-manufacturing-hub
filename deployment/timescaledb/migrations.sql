-- Add here your migrations

-- #2
ALTER TABLE configurationTable ADD IF NOT EXISTS AutomaticallyIdentifyChangeovers BOOLEAN DEFAULT true;