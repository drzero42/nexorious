CREATE TABLE smell_ignores (
    id text NOT NULL,
    user_id text NOT NULL,
    user_game_id text NOT NULL,
    check_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY smell_ignores
    ADD CONSTRAINT smell_ignores_pkey PRIMARY KEY (id);
ALTER TABLE ONLY smell_ignores
    ADD CONSTRAINT smell_ignores_user_game_check_key UNIQUE (user_id, user_game_id, check_id);
ALTER TABLE ONLY smell_ignores
    ADD CONSTRAINT smell_ignores_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY smell_ignores
    ADD CONSTRAINT smell_ignores_user_game_id_fkey FOREIGN KEY (user_game_id)
        REFERENCES user_games(id) ON DELETE CASCADE;

CREATE INDEX smell_ignores_user_check_idx ON smell_ignores (user_id, check_id);
