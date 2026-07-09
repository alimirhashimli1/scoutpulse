CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE leagues (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL UNIQUE,
    country TEXT NOT NULL
);

CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL UNIQUE,
    league_id UUID NOT NULL,
    FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE RESTRICT
);

CREATE TABLE coaches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    date_of_birth DATE NOT NULL,
    nationality TEXT NOT NULL,
    team_id UUID,
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE SET NULL
);

CREATE TABLE players (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    date_of_birth DATE NOT NULL,
    nationality TEXT NOT NULL,
    position TEXT NOT NULL,
    team_id UUID,
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE SET NULL
);
