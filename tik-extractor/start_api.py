"""
Startup script để chạy TikTokDownloader ở Web API mode tự động.
Bypass interactive menu, tiếp nhận disclaimer và chạy thẳng server.
"""
import asyncio

from src.application import TikTokDownloader
from src.config import Parameter, Settings
from src.custom import PROJECT_ROOT, SERVER_HOST, SERVER_PORT
from src.manager import Database, DownloadRecorder
from src.record import BaseLogger
from src.tools import ColorfulConsole
from src.module import Cookie
from src.translation import switch_language

from src.application.main_server import APIServer


async def main():
    db = Database()
    await db.__aenter__()

    # Accept disclaimer + set language
    await db.update_config_data("Disclaimer", 1)
    await db.update_option_data("Language", "en_US")
    switch_language("en_US")

    console = ColorfulConsole(debug=True)
    settings = Settings(PROJECT_ROOT, console)
    cookie = Cookie(settings, console)

    # Read config
    config_list = await db.read_config_data()
    config = {i["NAME"]: i["VALUE"] for i in config_list}

    recorder = DownloadRecorder(db, config.get("Record", 0), console)
    logger = BaseLogger

    parameter = Parameter(
        settings,
        cookie,
        logger=logger,
        console=console,
        **settings.read(),
        recorder=recorder,
    )
    parameter.set_headers_cookie()

    console.print(f"🚀 Starting TikTokDownloader Web API on http://{SERVER_HOST}:{SERVER_PORT}")
    console.print(f"📖 Docs: http://0.0.0.0:{SERVER_PORT}/docs")

    try:
        await APIServer(parameter, db).run_server(SERVER_HOST, SERVER_PORT)
    finally:
        await parameter.close_client()
        await db.__aexit__(None, None, None)


if __name__ == "__main__":
    asyncio.run(main())
