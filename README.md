# 📸 PancyScreenAPI (PancyScreenShots)

![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)
![Node Version](https://img.shields.io/badge/Node.js-18+-339933?style=flat&logo=node.js)
![TensorFlow](https://img.shields.io/badge/TensorFlow.js-FF6F00?style=flat&logo=tensorflow)
![License](https://img.shields.io/badge/License-MIT-blue.svg)

**PancyScreenAPI** es un potente microservicio de captura de pantalla y análisis de Inteligencia Artificial diseñado para entornos de alto rendimiento (como Discord Bots). Combina la velocidad de **Go (Fiber)** para el enrutamiento y procesamiento concurrente de navegadores *headless*, con el poder de **Node.js y TensorFlow** para la clasificación de contenido.

## ✨ Características Principales

*   🚀 **Cola de Trabajo Asíncrona:** Arquitectura *Worker Pool* en Go que maneja múltiples instancias de Chrome (`chromedp`) sin saturar la memoria RAM.
*   🛡️ **Seguridad Extrema:**
    *   Protección SSRF incorporada (Bloqueo de peticiones a `localhost` e IPs privadas).
    *   Bloqueo de descargas de archivos para evitar vulnerabilidades de inyección.
    *   Integración DNS Nativa con `CleanBrowsing (1.0.0.3)` para bloquear dominios maliciosos a nivel de red.
*   🧠 **Microservicio de Inteligencia Artificial (TensorFlow):**
    *   **Imágenes NSFW:** Modelo `InceptionV3` (nsfwjs) para detectar contenido explícito con alta precisión.
    *   **Texto Tóxico:** Modelo oficial de Google `@tensorflow-models/toxicity` para analizar mensajes y detectar insultos/toxicidad.
    *   **Detector Anti-Scam:** Modelo de red neuronal personalizado entrenado para detectar enlaces de *phishing* (Scam, falsos regalos de Nitro, etc.).
*   ⚡ **Optimizaciones de Producción:** Limitador de peticiones (Rate Limit), sistema de Caché en memoria de 30 minutos y endpoint de `/health`.

## ⚙️ Requisitos

*   [Go](https://golang.org/dl/) 1.22 o superior
*   [Node.js](https://nodejs.org/) v18+ y npm
*   Google Chrome (o Chromium) instalado en el sistema

## 🚀 Instalación y Despliegue

1. **Clonar el Repositorio**
   ```bash
   git clone https://github.com/PancyStudios/PancyScreenShots.git
   cd PancyScreenShots
   ```

2. **Configurar Variables de Entorno**
   Crea un archivo `.env` en la raíz del proyecto:
   ```env
   PORT=3000
   AUTH_SCREENSHOTS=tu_token_secreto_aqui
   ```

3. **Instalar Dependencias de IA (Node.js)**
   ```bash
   cd nsfw-detector
   npm install --legacy-peer-deps
   # Opcional: Reentrenar el modelo Anti-Scam
   # node train_scam.js
   cd ..
   ```

4. **Compilar y Ejecutar (Go)**
   Usa nuestro script automatizado para compilar tu binario inyectando metadatos de versión:
   ```bash
   ./build.sh amd64   # Para procesadores x86_64
   # ./build.sh arm64 # Para procesadores ARM
   
   ./PancyScreenAPI.x86_64
   ```

> [!NOTE]
> Al iniciar el binario en Go, este levantará automáticamente el microservicio de IA en Node.js en segundo plano (puerto 3001).

## 📡 Endpoints de la API

Todas las peticiones a rutas privadas requieren el header `Authorization: Bearer <tu_token_secreto>`.

| Ruta | Método | Descripción |
| :--- | :---: | :--- |
| `/health` | `GET` | Muestra el estado del sistema, versión y fecha de compilación. (Pública) |
| `/api/private/screenshot/sfw` | `POST` | Toma captura de una URL. Filtra NSFW por DNS. |
| `/api/private/screenshot/nsfw` | `POST` | Toma captura de una URL sin filtrado estricto. |
| `/api/private/ai/nsfw` | `POST` | Analiza un archivo binario de imagen y detecta si es NSFW/SFW. |
| `/api/private/ai/toxicity` | `POST` | Analiza toxicidad de texto (Body JSON con `{"text": "..."}`). |
| `/api/private/ai/scam` | `POST` | Analiza probabilidad de Phishing (Body JSON con `{"url": "..."}`). |

## 🤝 Contribuir

¡Las contribuciones son bienvenidas! PancyScreenAPI es de código abierto. Siéntete libre de hacer un *fork*, crear tus propias ramas (*branches*) y abrir un *Pull Request*.

## 📄 Licencia

Este proyecto está bajo la Licencia MIT. Consulta el archivo [LICENSE](LICENSE) para más detalles.
