package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/joho/godotenv"
	"github.com/jung-kurt/gofpdf"
)

// Estructura para la denuncia
type Denuncia struct {
	TipoDocumento     string `json:"tipoDocumento"`
	NumDocumento      string `json:"numDocumento"`
	Nombres           string `json:"nombres"`
	Apellidos         string `json:"apellidos"`
	Email             string `json:"email"`
	Telefono          string `json:"telefono"`
	TipoDenuncia      string `json:"tipoDenuncia"`
	Direccion         string `json:"direccion"`
	Descripcion       string `json:"descripcion"`
	CodigoSeguimiento string `json:"codigoSeguimiento"`
	FechaRegistro     string `json:"fechaRegistro"`
}

func main() {
	godotenv.Load()

	mux := http.NewServeMux()

	// archivos estáticos (CSS, JS, imágenes)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// SERVIR ARCHIVOS PDF
	mux.Handle("/denuncias/", http.StripPrefix("/denuncias/", http.FileServer(http.Dir("denuncias"))))

	// Ruta principal
	mux.HandleFunc("/", handleIndex)

	// Ruta API
	mux.HandleFunc("/api/denuncias", handleEnviarDenuncia)

	// pdf stuff
	mux.HandleFunc("/descargar/", handleDescargarPDF)

	//confirmacion
	mux.HandleFunc("/confirmacion", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "templates/confirmacion.html")
	})

	log.Println("Servidor iniciando en http://localhost:8081")
	log.Println("Sistema de Denuncias Ciudadanas - gob.pe")
	log.Println("Accede a: http://localhost:8081")

	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Fatal(err)
	}

}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		log.Println("Error al parsear template:", err)
		http.Error(w, "Error interno del servidor", 500)
		return
	}

	err = tmpl.Execute(w, nil)
	if err != nil {
		log.Println("Error al ejecutar template:", err)
		http.Error(w, "Error interno del servidor", 500)
	}
}

func handleEnviarDenuncia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var denuncia Denuncia
	if err := json.NewDecoder(r.Body).Decode(&denuncia); err != nil {
		log.Println("Error al decodificar JSON:", err)
		http.Error(w, "Error al procesar los datos", 400)
		return
	}

	// Generar código de seguimiento
	denuncia.CodigoSeguimiento = "DEN-" + time.Now().Format("20060102150405")
	denuncia.FechaRegistro = time.Now().Format("02/01/2006 15:04")

	// Log en servidor
	log.Printf("NUEVA DENUNCIA RECIBIDA:")
	log.Printf("   Código: %s", denuncia.CodigoSeguimiento)
	log.Printf("   Nombre: %s %s", denuncia.Nombres, denuncia.Apellidos)
	log.Printf("   Tipo: %s", denuncia.TipoDenuncia)
	log.Printf("   Descripción: %s", denuncia.Descripcion)
	log.Println("   ---")

	//generar pdf
	rutaPDF, err := generarPDF(denuncia)
	if err != nil {
		log.Println("Error al generar PDF:", err)
	}

	// Responder
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Denuncia registrada exitosamente",
		"data": map[string]string{
			"codigoSeguimiento": denuncia.CodigoSeguimiento,
			"fechaRegistro":     denuncia.FechaRegistro,
			"pdf":               "/" + rutaPDF,
		},
	})

	if denuncia.Nombres == "" {
		denuncia.Nombres = "Anónimo(a)"
	}
	if denuncia.NumDocumento == "" {
		denuncia.NumDocumento = "N/A"
	}
	if denuncia.Email == "" {
		denuncia.Email = "N/A"
	}
	if denuncia.Direccion == "" {
		denuncia.Direccion = "Sin especificar"
	}

	datos := extraerDatosRegex(denuncia.Descripcion, denuncia.Nombres)

	//gorutines
	resultCh := make(chan string)
	errCh := make(chan error)

	go llamarGroqDenuncia(denuncia.Descripcion, datos, resultCh, errCh)

	var denunciaFormal string
	select {
	case denunciaFormal = <-resultCh:
		log.Println("Se ha generado la denuncia formal.")
	case err := <-errCh:
		log.Println("Error en LLM:", err)
		denunciaFormal = "No se pudo generar denuncia formal."
	}

	err = os.WriteFile("denuncias/"+denuncia.CodigoSeguimiento+".txt", []byte(denunciaFormal), 0644)
	if err != nil {
		log.Println("Error guardando archivo:", err)
	} else {
		log.Println("Denuncia guardada localmente.")
	}

}

func handleDescargarPDF(w http.ResponseWriter, r *http.Request) {
	archivo := r.URL.Path[len("/descargar/"):]
	ruta := "denuncias/" + archivo

	http.ServeFile(w, r, ruta)
}

func extraerDatosRegex(desc string, nombres string) map[string]string {
	datos := make(map[string]string)

	// Ejemplo simple
	rLugar := regexp.MustCompile(`en\s+([A-Za-z\s]+)`)
	if m := rLugar.FindStringSubmatch(desc); len(m) > 1 {
		datos["lugar"] = m[1]
	} else {
		datos["lugar"] = "No especificado"
	}

	rHora := regexp.MustCompile(`\b\d{1,2}:\d{2}\b`)
	if m := rHora.FindString(desc); m != "" {
		datos["hora"] = m
	} else {
		datos["hora"] = "No especificada"
	}

	rDelito := regexp.MustCompile(`(?i)(robo|asalto|hurto|agresión|violencia)`)
	if m := rDelito.FindString(desc); m != "" {
		datos["delito"] = m
	} else {
		datos["delito"] = "No identificado"
	}

	datos["victima"] = nombres
	datos["agresor"] = "No especificado"

	return datos
}

func llamarGroqDenuncia(descripcion string, datosExtraidos map[string]string, ch chan<- string, errCh chan<- error) {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		errCh <- fmt.Errorf("GROQ_API_KEY no está definida")
		return
	}

	prompt := fmt.Sprintf(`Convierte de lenguaje natural a denuncia formal. 
**No uses tildes ni diéresis** (á, é, í, ó, ú, ü), reemplaza ñ por n. 
Incluye: victima, agresor, lugar, hora, delito, descripción.
Datos extraidos: Victima: %s Agresor: %s Lugar: %s Hora: %s Delito: %s 
Descripcion del denunciante: %s`,
		datosExtraidos["victima"], datosExtraidos["agresor"], datosExtraidos["lugar"],
		datosExtraidos["hora"], datosExtraidos["delito"], descripcion)

	payload := map[string]interface{}{
		"model": "llama-3.3-70b-versatile",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		errCh <- err
		return
	}

	req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		errCh <- err
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		errCh <- err
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var groqResp GroqResponse
	if err := json.Unmarshal(body, &groqResp); err != nil {
		errCh <- err
		return
	}

	ch <- groqResp.Choices[0].Message.Content
}

type GroqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func generarPDF(denuncia Denuncia) (string, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.AddPage()

	pdf.AddUTF8Font("Arial", "", "fonts/arial.ttf")
	pdf.SetFont("Arial", "", 12)

	// encabezado
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "REPUBLICA DEL PERU", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 6, "Sistema de Denuncias Ciudadanas Seguras", "", 1, "C", false, 0, "")
	pdf.Ln(5)

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "DENUNCIA CIUDADANA", "", 1, "C", false, 0, "")
	pdf.Ln(3)

	// Línea divisoria
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(5)

	// ========= DATOS PRINCIPALES =========
	pdf.SetFont("Arial", "", 12)
	pdf.MultiCell(0, 7, fmt.Sprintf(
		"Codigo de Seguimiento: %s\nFecha de Registro: %s",
		denuncia.CodigoSeguimiento,
		denuncia.FechaRegistro,
	), "", "L", false)
	pdf.Ln(3)

	// ========= SECCIÓN: DENUNCIANTE =========
	pdf.SetFont("Arial", "B", 13)
	pdf.Cell(0, 8, "I. Datos del Denunciante")
	pdf.Ln(9)
	pdf.SetFont("Arial", "", 12)

	pdf.MultiCell(0, 7, fmt.Sprintf(
		"Nombre Completo: %s %s\nTipo de Documento: %s\nNumero de Documento: %s\nCorreo Electronico: %s\nTelefono: %s",
		denuncia.Nombres,
		denuncia.Apellidos,
		denuncia.TipoDocumento,
		denuncia.NumDocumento,
		denuncia.Email,
		denuncia.Telefono,
	), "", "L", false)
	pdf.Ln(3)

	// ========= SECCIÓN: DETALLE DE LA DENUNCIA =========
	pdf.SetFont("Arial", "B", 13)
	pdf.Cell(0, 8, "II. Detalles de la Denuncia")
	pdf.Ln(9)
	pdf.SetFont("Arial", "", 12)

	pdf.MultiCell(0, 7, fmt.Sprintf(
		"Tipo de Denuncia: %s\nDireccion del incidente: %s\n\nDescripcion completa de los hechos:\n%s",
		denuncia.TipoDenuncia,
		denuncia.Direccion,
		denuncia.Descripcion,
	), "", "L", false)
	pdf.Ln(5)

	// Línea final
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(5)

	// ========= PIE DE PÁGINA =========
	pdf.SetFont("Arial", "I", 10)
	pdf.MultiCell(0, 6,
		"Este documento es una constancia generada automaticamente por el Sistema de Denuncias Ciudadanas.\n"+
			"Mantenga su codigo de seguimiento para futuras consultas.",
		"", "C", false)

	// Guardar archivo
	fileName := fmt.Sprintf("denuncias/%s.pdf", denuncia.CodigoSeguimiento)
	err := pdf.OutputFileAndClose(fileName)
	if err != nil {
		return "", err
	}

	return fileName, nil
}
