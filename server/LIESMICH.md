# Turmuhr-Server

Kleines Programm ohne Installation, das die Grimoire-App im lokalen Netz ausliefert und
Spieler-Handys sowie die Soundanlage mit dem Handy des Spielleiters verbindet.

**Download:** [Windows](https://github.com/Niztra/turmuhr-grimoire/releases/download/server/turmuhr-server-windows.exe) ·
[Linux](https://github.com/Niztra/turmuhr-grimoire/releases/download/server/turmuhr-server-linux)

## Am Spieleabend

1. Programm auf dem PC mit den Lautsprechern starten.
   - Windows: Doppelklick. SmartScreen warnt bei unbekannten Programmen: „Weitere Informationen“ → „Trotzdem ausführen“.
     Fragt die Firewall, den Zugriff im **privaten Netzwerk** erlauben.
   - Linux: `chmod +x turmuhr-server-linux && ./turmuhr-server-linux`
2. Der Browser öffnet sich mit einem QR-Code. Der Spielleiter scannt ihn mit dem Handy und macht dort als Spielleiter weiter.
3. Am PC „Dieser PC ist die Soundanlage“ wählen.
4. Spielleiter: Aufbau → Spieler-Handys → Spielraum öffnen. Die Spieler scannen den dort gezeigten QR-Code.

## Netzwerk

Alle Geräte müssen im selben Netz sein wie der PC. Internet braucht es nicht.
Viele Gäste-WLANs blockieren Verbindungen zwischen Geräten. Dann einen Hotspot aufmachen
(Windows: Einstellungen → Mobiler Hotspot, oder am Handy) und PC und alle Handys damit verbinden.

## Was der Server weiß

Nur, was der Spielleiter freigibt: Skript, Namen, Leben/Tod, Phase, Ansage, Nominierungen,
und je zugeordnetem Handy dessen eigene Rolle und Nachrichten. Das Grimoire bleibt auf dem Handy des Spielleiters.
Alles liegt nur im Arbeitsspeicher und ist nach dem Beenden weg.

Optionen: `-port 8080` (Standard, nimmt sonst den nächsten freien), `-kein-browser`.

Bauen: `./build.sh` (braucht Go). Bettet `../index.html` und `../audio` ein.
