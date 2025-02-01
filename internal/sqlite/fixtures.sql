INSERT INTO cases(id, name, author, image_path)
VALUES ('rue-morgue', 'The Murders in the Rue Morgue', 'Edgar Allan Poe', '/images/rue_morgue.webp')
ON CONFLICT(id) DO UPDATE SET name       = excluded.name,
                              author     = excluded.author,
                              image_path = excluded.image_path;

INSERT INTO investigation_targets(id, name, short_name, type, image_path, case_id)
VALUES ('le-bon', 'Adolphe Le Bon', 'Adolphe', 'person', 'https://myrjola.twic.pics/petrapp/adolphe_le-bon.webp',
        'rue-morgue'),
       ('rue-morgue', 'Rue Morgue Murder Scene', 'Rue Morgue', 'scene',
        'https://myrjola.twic.pics/petrapp/rue-morgue.webp', 'rue-morgue')
ON CONFLICT (id) DO UPDATE SET name       = excluded.name,
                               short_name = excluded.short_name,
                               case_id    = excluded.case_id,
                               image_path = excluded.image_path;

INSERT INTO clues(id, description, keywords, investigation_target_id)
VALUES ('le-bon-victim-belongings',
        'The victims'' belongings in Adolphe''s posession were given to him as collateral for a debt.',
        'gold,watch,scissors', 'le-bon'),
       ('le-bon-last-meeting-with-the-victim',
        'Adolphe met the victims the day before the murder when he loaned them 4000 francs. Madame and Mademoiselle L''Espanaye relieved him of the money plaed in two bags. He then bowed and departed. Nobody else was seen during this interaction since it happened on a quiet street.',
        'victims,last-seen,loan', 'le-bon')
ON CONFLICT (id) DO UPDATE SET description             = excluded.description,
                               keywords                = excluded.keywords,
                               investigation_target_id = excluded.investigation_target_id;
