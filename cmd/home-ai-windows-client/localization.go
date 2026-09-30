package main

import (
	"fmt"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

const (
	uiLanguageEnglish = "en"
	uiLanguageRussian = "ru"
)

var uiText = map[string]map[string]string{
	uiLanguageEnglish: {},
	uiLanguageRussian: {},
}

func init() {
	add := func(language, key, value string) {
		uiText[language][key] = value
	}
	add(uiLanguageEnglish, "window_title", "Home-AI Windows Client")
	add(uiLanguageEnglish, "language", "Language")
	add(uiLanguageEnglish, "server", "Server")
	add(uiLanguageEnglish, "user", "User")
	add(uiLanguageEnglish, "password", "Password")
	add(uiLanguageEnglish, "password_hint", "blank = stored credential")
	add(uiLanguageEnglish, "connect_save", "Connect & save")
	add(uiLanguageEnglish, "remote_folder", "Remote folder")
	add(uiLanguageEnglish, "local_source", "Local source")
	add(uiLanguageEnglish, "browse", "Browse...")
	add(uiLanguageEnglish, "destination", "Destination")
	add(uiLanguageEnglish, "interval", "Interval")
	add(uiLanguageEnglish, "conflict", "Conflict")
	add(uiLanguageEnglish, "conflict_stop", "Stop")
	add(uiLanguageEnglish, "conflict_skip", "Skip")
	add(uiLanguageEnglish, "conflict_replace", "Trash + replace")
	add(uiLanguageEnglish, "add_profile", "Add profile")
	add(uiLanguageEnglish, "update_profile", "Update profile")
	add(uiLanguageEnglish, "new_clear", "New / clear")
	add(uiLanguageEnglish, "enable", "Enable")
	add(uiLanguageEnglish, "disable", "Disable")
	add(uiLanguageEnglish, "delete", "Delete")
	add(uiLanguageEnglish, "refresh", "Refresh")
	add(uiLanguageEnglish, "sync_profiles", "Sync profiles")
	add(uiLanguageEnglish, "autostart_checking", "Autostart: checking...")
	add(uiLanguageEnglish, "sync_now", "Sync now")
	add(uiLanguageEnglish, "enable_agent", "Enable agent")
	add(uiLanguageEnglish, "disable_agent", "Disable agent")
	add(uiLanguageEnglish, "status", "Status")
	add(uiLanguageEnglish, "ready", "Ready")
	add(uiLanguageEnglish, "connecting", "Connecting to Home-AI...")
	add(uiLanguageEnglish, "connection_failed", "Connection failed")
	add(uiLanguageEnglish, "account_required", "Server URL and username are required.")
	add(uiLanguageEnglish, "connected_writable", "Connected. %d writable folder(s) available.")
	add(uiLanguageEnglish, "invalid_interval", "Invalid interval. Use values such as 15m, 1h or 24h.")
	add(uiLanguageEnglish, "required_fields", "Server, user, remote folder and local source are required.")
	add(uiLanguageEnglish, "profile_saved", "Sync profile saved.")
	add(uiLanguageEnglish, "profile_on", "ON")
	add(uiLanguageEnglish, "profile_off", "OFF")
	add(uiLanguageEnglish, "never", "never")
	add(uiLanguageEnglish, "failed", "FAILED")
	add(uiLanguageEnglish, "editing_profile", "Editing profile %s")
	add(uiLanguageEnglish, "new_profile", "New profile.")
	add(uiLanguageEnglish, "select_profile", "Select a sync profile first.")
	add(uiLanguageEnglish, "profile_enabled", "Profile enabled.")
	add(uiLanguageEnglish, "profile_disabled", "Profile disabled.")
	add(uiLanguageEnglish, "delete_confirm", "Delete the selected sync profile?")
	add(uiLanguageEnglish, "profile_deleted", "Profile deleted.")
	add(uiLanguageEnglish, "refreshed", "Refreshed.")
	add(uiLanguageEnglish, "sync_requested", "Sync requested through the running Home-AI agent.")
	add(uiLanguageEnglish, "syncing", "Syncing enabled profiles...")
	add(uiLanguageEnglish, "manual_sync_finished", "Manual sync finished. %d profile(s) processed.")
	add(uiLanguageEnglish, "sync_failed", "Sync failed")
	add(uiLanguageEnglish, "enabling_agent", "Enabling Home-AI background agent...")
	add(uiLanguageEnglish, "agent_enabled", "Background agent enabled and started.")
	add(uiLanguageEnglish, "disabling_agent", "Disabling Home-AI background agent...")
	add(uiLanguageEnglish, "agent_disabled", "Background agent autostart disabled.")
	add(uiLanguageEnglish, "agent_error", "Agent error")
	add(uiLanguageEnglish, "autostart_error", "Autostart status error")
	add(uiLanguageEnglish, "autostart_on", "ON")
	add(uiLanguageEnglish, "autostart_off", "OFF")
	add(uiLanguageEnglish, "agent_running", "running")
	add(uiLanguageEnglish, "agent_stopped", "stopped")
	add(uiLanguageEnglish, "autostart_state", "Autostart: %s  |  Agent: %s")
	add(uiLanguageEnglish, "browse_title", "Select local folder to sync")
	add(uiLanguageEnglish, "browse_no_path", "Windows did not return a filesystem path for the selected folder.")
	add(uiLanguageEnglish, "saved_folder", "Saved folder")
	add(uiLanguageEnglish, "profiles_unavailable", "profiles unavailable")
	add(uiLanguageEnglish, "no_enabled_profiles", "no enabled profiles")
	add(uiLanguageEnglish, "not_synced_yet", "not synced yet")
	add(uiLanguageEnglish, "summary_enabled_new", "%d enabled · not synced yet")
	add(uiLanguageEnglish, "summary_last", "%d enabled · last %s %s")
	add(uiLanguageEnglish, "sync_failed_title", "Home-AI sync failed")
	add(uiLanguageEnglish, "open_log_details", "Open the agent log for details.")
	add(uiLanguageEnglish, "sync_complete_title", "Home-AI sync complete")
	add(uiLanguageEnglish, "profiles_processed", "%d profile(s) processed.")
	add(uiLanguageEnglish, "tray_name", "Home-AI Sync Agent")
	add(uiLanguageEnglish, "settings", "Settings")
	add(uiLanguageEnglish, "open_log", "Open log")
	add(uiLanguageEnglish, "open_sync_profiles", "Open sync profiles")
	add(uiLanguageEnglish, "exit", "Exit")
	add(uiLanguageEnglish, "error_prefix", "Error")
	add(uiLanguageRussian, "window_title", "Home-AI — Windows клиент")
	add(uiLanguageRussian, "language", "Язык")
	add(uiLanguageRussian, "server", "Сервер")
	add(uiLanguageRussian, "user", "Пользователь")
	add(uiLanguageRussian, "password", "Пароль")
	add(uiLanguageRussian, "password_hint", "пусто = сохранённый пароль")
	add(uiLanguageRussian, "connect_save", "Подключить и сохранить")
	add(uiLanguageRussian, "remote_folder", "Папка на сервере")
	add(uiLanguageRussian, "local_source", "Локальная папка")
	add(uiLanguageRussian, "browse", "Выбрать...")
	add(uiLanguageRussian, "destination", "Путь назначения")
	add(uiLanguageRussian, "interval", "Интервал")
	add(uiLanguageRussian, "conflict", "Конфликт")
	add(uiLanguageRussian, "conflict_stop", "Остановить")
	add(uiLanguageRussian, "conflict_skip", "Пропустить")
	add(uiLanguageRussian, "conflict_replace", "В корзину + заменить")
	add(uiLanguageRussian, "add_profile", "Добавить профиль")
	add(uiLanguageRussian, "update_profile", "Сохранить профиль")
	add(uiLanguageRussian, "new_clear", "Новый / очистить")
	add(uiLanguageRussian, "enable", "Включить")
	add(uiLanguageRussian, "disable", "Выключить")
	add(uiLanguageRussian, "delete", "Удалить")
	add(uiLanguageRussian, "refresh", "Обновить")
	add(uiLanguageRussian, "sync_profiles", "Профили синхронизации")
	add(uiLanguageRussian, "autostart_checking", "Автозапуск: проверка...")
	add(uiLanguageRussian, "sync_now", "Синхронизировать")
	add(uiLanguageRussian, "enable_agent", "Включить агент")
	add(uiLanguageRussian, "disable_agent", "Выключить агент")
	add(uiLanguageRussian, "status", "Состояние")
	add(uiLanguageRussian, "ready", "Готово")
	add(uiLanguageRussian, "connecting", "Подключение к Home-AI...")
	add(uiLanguageRussian, "connection_failed", "Ошибка подключения")
	add(uiLanguageRussian, "account_required", "Укажите адрес сервера и имя пользователя.")
	add(uiLanguageRussian, "connected_writable", "Подключено. Доступно папок для записи: %d.")
	add(uiLanguageRussian, "invalid_interval", "Некорректный интервал. Используйте, например, 15m, 1h или 24h.")
	add(uiLanguageRussian, "required_fields", "Нужно указать сервер, пользователя, папку на сервере и локальную папку.")
	add(uiLanguageRussian, "profile_saved", "Профиль синхронизации сохранён.")
	add(uiLanguageRussian, "profile_on", "ВКЛ")
	add(uiLanguageRussian, "profile_off", "ВЫКЛ")
	add(uiLanguageRussian, "never", "ещё не запускался")
	add(uiLanguageRussian, "failed", "ОШИБКА")
	add(uiLanguageRussian, "editing_profile", "Редактируется профиль %s")
	add(uiLanguageRussian, "new_profile", "Новый профиль.")
	add(uiLanguageRussian, "select_profile", "Сначала выберите профиль синхронизации.")
	add(uiLanguageRussian, "profile_enabled", "Профиль включён.")
	add(uiLanguageRussian, "profile_disabled", "Профиль выключен.")
	add(uiLanguageRussian, "delete_confirm", "Удалить выбранный профиль синхронизации?")
	add(uiLanguageRussian, "profile_deleted", "Профиль удалён.")
	add(uiLanguageRussian, "refreshed", "Обновлено.")
	add(uiLanguageRussian, "sync_requested", "Синхронизация передана запущенному агенту Home-AI.")
	add(uiLanguageRussian, "syncing", "Синхронизация включённых профилей...")
	add(uiLanguageRussian, "manual_sync_finished", "Ручная синхронизация завершена. Обработано профилей: %d.")
	add(uiLanguageRussian, "sync_failed", "Ошибка синхронизации")
	add(uiLanguageRussian, "enabling_agent", "Включение фонового агента Home-AI...")
	add(uiLanguageRussian, "agent_enabled", "Фоновый агент включён и запущен.")
	add(uiLanguageRussian, "disabling_agent", "Выключение фонового агента Home-AI...")
	add(uiLanguageRussian, "agent_disabled", "Автозапуск фонового агента выключен.")
	add(uiLanguageRussian, "agent_error", "Ошибка агента")
	add(uiLanguageRussian, "autostart_error", "Ошибка проверки автозапуска")
	add(uiLanguageRussian, "autostart_on", "ВКЛ")
	add(uiLanguageRussian, "autostart_off", "ВЫКЛ")
	add(uiLanguageRussian, "agent_running", "запущен")
	add(uiLanguageRussian, "agent_stopped", "остановлен")
	add(uiLanguageRussian, "autostart_state", "Автозапуск: %s  |  Агент: %s")
	add(uiLanguageRussian, "browse_title", "Выберите локальную папку для синхронизации")
	add(uiLanguageRussian, "browse_no_path", "Windows не вернула путь файловой системы для выбранной папки.")
	add(uiLanguageRussian, "saved_folder", "Сохранённая папка")
	add(uiLanguageRussian, "profiles_unavailable", "профили недоступны")
	add(uiLanguageRussian, "no_enabled_profiles", "нет включённых профилей")
	add(uiLanguageRussian, "not_synced_yet", "синхронизация ещё не запускалась")
	add(uiLanguageRussian, "summary_enabled_new", "%d включено · синхронизации ещё не было")
	add(uiLanguageRussian, "summary_last", "%d включено · последний результат %s %s")
	add(uiLanguageRussian, "sync_failed_title", "Ошибка синхронизации Home-AI")
	add(uiLanguageRussian, "open_log_details", "Откройте журнал агента для подробностей.")
	add(uiLanguageRussian, "sync_complete_title", "Синхронизация Home-AI завершена")
	add(uiLanguageRussian, "profiles_processed", "Обработано профилей: %d.")
	add(uiLanguageRussian, "tray_name", "Home-AI — Агент синхронизации")
	add(uiLanguageRussian, "settings", "Настройки")
	add(uiLanguageRussian, "open_log", "Открыть журнал")
	add(uiLanguageRussian, "open_sync_profiles", "Открыть профили синхронизации")
	add(uiLanguageRussian, "exit", "Выход")
	add(uiLanguageRussian, "error_prefix", "Ошибка")
}

func normalizeUILanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "ru", "ru-ru":
		return uiLanguageRussian
	case "en", "en-us", "en-gb":
		return uiLanguageEnglish
	default:
		return uiLanguageEnglish
	}
}

func textForLanguage(language, key string) string {
	language = normalizeUILanguage(language)
	if value := uiText[language][key]; value != "" {
		return value
	}
	return uiText[uiLanguageEnglish][key]
}

func textForLanguagef(language, key string, args ...any) string {
	return fmt.Sprintf(textForLanguage(language, key), args...)
}

func preferredUILanguage() string {
	path, err := defaultClientSettingsPath()
	if err != nil {
		return defaultUILanguage()
	}
	return preferredUILanguageForSettingsPath(path)
}

func preferredUILanguageForSettingsPath(path string) string {
	settings, err := windowsclient.LoadClientSettings(path)
	if err != nil || settings.Language == "" {
		return defaultUILanguage()
	}
	return normalizeUILanguage(settings.Language)
}
