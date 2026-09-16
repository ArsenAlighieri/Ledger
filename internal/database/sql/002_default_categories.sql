-- 002_default_categories.sql
INSERT INTO categories (name, type, icon, is_default) VALUES
    ('Konut', 'expense', 'home', 1),
    ('Market', 'expense', 'shopping-cart', 1),
    ('Dışarıda Yemek', 'expense', 'utensils', 1),
    ('Ulaşım', 'expense', 'bus', 1),
    ('Araç / Yakıt', 'expense', 'car', 1),
    ('Faturalar', 'expense', 'zap', 1),
    ('Abonelikler', 'expense', 'repeat', 1),
    ('Sosyal', 'expense', 'users', 1),
    ('Eğlence', 'expense', 'film', 1),
    ('Alışveriş', 'expense', 'shopping-bag', 1),
    ('Sağlık', 'expense', 'heart-pulse', 1),
    ('Eğitim', 'expense', 'book-open', 1),
    ('Seyahat', 'expense', 'plane', 1),
    ('Diğer', 'expense', 'more-horizontal', 1);

INSERT INTO categories (name, type, icon, is_default) VALUES
    ('Maaş', 'income', 'briefcase', 1),
    ('Burs', 'income', 'graduation-cap', 1),
    ('Aile Desteği', 'income', 'home', 1),
    ('Freelance', 'income', 'laptop', 1),
    ('Yatırım Geliri', 'income', 'trending-up', 1),
    ('Diğer', 'income', 'plus-circle', 1);
