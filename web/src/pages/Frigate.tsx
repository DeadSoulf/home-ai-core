import {useCallback, useState} from "react";
import {api} from "../api/client";
import {useResource} from "../hooks/useResource";
import {ErrorNotice} from "../components/ErrorNotice";
import {LoadingState, Panel} from "../components/Panel";
import {PageHeading} from "./Dashboard";
import {frigateUIURL} from "../ui/frigate";

// Render the ORIGINAL application, not a subset of its APIs. Different origin
// plus sandbox keeps upstream scripts away from Core cookies/DOM/localStorage.
export function FrigatePage({onNavigate}: {onNavigate: (path: string) => void}) {
  const load = useCallback((signal: AbortSignal) => api.frigateUI(signal), []);
  const gateway = useResource(load, 0, 15000);
  const [revision, setRevision] = useState(0);
  let address: string | undefined;
  let addressError: unknown;
  if (gateway.data?.available && gateway.data.url) {
    try {address = frigateUIURL(window.location.href, gateway.data.url);}
    catch (error) {addressError = error;}
  }
  return <div className="page frigate-page frigate-full-page">
    <PageHeading title="Frigate" subtitle="Полный официальный интерфейс: прямой эфир, архив, события и настройки" />
    <div className="button-row frigate-toolbar">
      <button type="button" className="button secondary" onClick={() => onNavigate("/modules")}>Модули и обновления</button>
      <button type="button" className="button secondary" onClick={() => {setRevision(value => value + 1); gateway.reload();}}>Перезагрузить интерфейс</button>
      {address && <a className="button primary" href={address} target="_blank" rel="noopener noreferrer">Открыть Frigate в отдельной вкладке</a>}
    </div>
    {gateway.loading && !gateway.data && <LoadingState />}
    {gateway.error && <ErrorNotice error={gateway.error} />}
    {addressError !== undefined && <ErrorNotice error={addressError} />}
    {gateway.data && !gateway.data.available && <Panel title="Интерфейс Frigate недоступен">
      <p role="alert">{gateway.data.error || "Защищённый интерфейс Frigate ещё не запущен."}</p>
      <p>Проверьте, что модуль запущен, а порт HTTPS-интерфейса (по умолчанию 8971) свободен. Настройка сервера: HOME_AI_FRIGATE_UI_LISTEN.</p>
    </Panel>}
    {address && <>
      <details className="frigate-connection-help">
        <summary>Подключение и безопасность</summary>
        <p>Открыт штатный интерфейс официального контейнера Frigate. Вход защищён сессией HomeAiCore; пароли и токены Core в контейнер не передаются.</p>
        <p>Если браузер блокирует загрузку, откройте Frigate в отдельной вкладке и проверьте сертификат сервера. Для уведомлений, установки приложения и разрешения микрофона используйте отдельную вкладку.</p>
        <p>Обычный live-просмотр использует MSE/WebSocket через этот же защищённый интерфейс. Для WebRTC/двустороннего звука нужны доступный медиамаршрут и настройки go2rtc; аппаратные детекторы требуют соответствующих устройств.</p>
      </details>
      <iframe key={revision} className="frigate-official-frame" title="Полный интерфейс Frigate" src={address}
        sandbox="allow-same-origin allow-scripts allow-forms allow-downloads allow-modals allow-popups allow-popups-to-escape-sandbox"
        allow="autoplay; fullscreen; microphone; clipboard-write" allowFullScreen referrerPolicy="no-referrer" />
    </>}
  </div>;
}
