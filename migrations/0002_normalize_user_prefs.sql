-- +goose Up

-- +goose StatementBegin
UPDATE users SET unit_pref = 'lb_in' WHERE unit_pref IS NULL OR unit_pref = '';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET unit_pref = 'kg_cm' WHERE lower(unit_pref) = 'kg' OR lower(unit_pref) = 'metric';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET unit_pref = 'lb_in' WHERE lower(unit_pref) = 'lb' OR lower(unit_pref) = 'imperial';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET distance_unit_pref = 'mi' WHERE distance_unit_pref IS NULL OR distance_unit_pref = '';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET distance_unit_pref = 'km' WHERE lower(distance_unit_pref) IN ('km', 'kilometer', 'kilometers', 'kilometre', 'kilometres');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET distance_unit_pref = 'mi' WHERE lower(distance_unit_pref) IN ('mi', 'mile', 'miles');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET distance_unit_pref = CASE WHEN unit_pref = 'kg_cm' THEN 'km' ELSE 'mi' END WHERE lower(distance_unit_pref) NOT IN ('mi', 'km');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET theme_pref = 'light' WHERE theme_pref IS NULL OR theme_pref = '';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET theme_pref = lower(theme_pref);
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET theme_pref = 'dark_hc' WHERE theme_pref IN ('dark-hc', 'darkhc', 'high-contrast', 'high_contrast');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET theme_pref = 'peachpuff' WHERE theme_pref IN ('peach', 'vim-peachpuff');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET theme_pref = 'default' WHERE theme_pref IN ('vim', 'vimdefault');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET theme_pref = 'light' WHERE theme_pref NOT IN ('light', 'dark', 'forest', 'sunset', 'peachpuff', 'dark_hc', 'blue', 'darkblue', 'default', 'delek', 'desert', 'elflord', 'evening', 'habamax', 'industry', 'koehler', 'lunaperche', 'morning', 'murphy', 'pablo', 'quiet', 'retrobox', 'ron', 'shine', 'slate', 'sorbet', 'torte', 'wildcharm', 'zaibatsu', 'zellner');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET keep_awake = 1 WHERE keep_awake IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE users SET keep_awake = CASE WHEN keep_awake IN (0, 1) THEN keep_awake ELSE 1 END;
-- +goose StatementEnd

-- +goose Down
-- no-op: data normalizations are not reversible
