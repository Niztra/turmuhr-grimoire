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
2. Der Browser öffnet sich als Soundanlage und zeigt einen **6-stelligen Kopplungscode**. Einmal „Ton freischalten“ klicken (geht auch per Fernsteuerung).
3. Am Spielleiter-Handy (normale App) unter Aufbau → „Mit dem PC koppeln“ den Code eingeben. Das Handy wechselt zur App auf dem PC, nimmt das Spiel mit und öffnet den Spielraum.
4. Einladungslink und QR-Code stehen dann am Handy unter Aufbau: kopieren, per WhatsApp schicken oder zeigen.

**Wie das Handy den PC findet:** Mit Internet meldet die PC-Seite den Code bei einem kostenlosen Vermittlungsdienst (PeerJS) an, das Handy fragt dort nach der Adresse. Ohne Internet steckt die Adresse im Code selbst, wenn der PC in einem üblichen Netz hängt (192.168.x.x oder iPhone-Hotspot) und Port 8080 frei ist. Codes, die mit 3 bis 9 beginnen, gehen nur mit Internet.

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
