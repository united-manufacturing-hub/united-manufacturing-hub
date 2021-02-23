-- Add here your migrations

-- #2
ALTER TABLE configurationTable ADD IF NOT EXISTS AutomaticallyIdentifyChangeovers BOOLEAN DEFAULT true;

-- #74
ALTER TABLE counttable ADD IF NOT EXISTS scrap INTEGER DEFAULT 0;