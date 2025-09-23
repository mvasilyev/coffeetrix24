-- Таблица с токеном бота (для соответствия требованию: отдельная таблица)
CREATE TABLE IF NOT EXISTS bot_credentials (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    token TEXT NOT NULL
);

-- Общие настройки (ежедневное время рассылки, в формате HH:MM, UTC по умолчанию)
CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    daily_time TEXT NOT NULL DEFAULT '09:00' -- HH:MM
);

-- Чаты, где установлен бот
CREATE TABLE IF NOT EXISTS chats (
    chat_id INTEGER PRIMARY KEY,
    title TEXT,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Сессии дневных наборов участников (по чату и дате)
CREATE TABLE IF NOT EXISTS daily_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id INTEGER NOT NULL,
    session_date TEXT NOT NULL, -- YYYY-MM-DD
    invite_message_id INTEGER,  -- message id приглашения
    signup_deadline TIMESTAMP,  -- крайний срок набора (плюс 30 минут)
    closed INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(chat_id, session_date)
);

-- Участники текущего набора
CREATE TABLE IF NOT EXISTS participants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    username TEXT,
    display_name TEXT,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(session_id, user_id)
);
