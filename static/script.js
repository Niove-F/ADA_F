function toggleAnonMode() {
    const anonActive = document.getElementById("anonSwitch").checked;
    const datos = document.getElementById("datosDenunciante");

    const inputs = datos.querySelectorAll("input, select");
    inputs.forEach(el => {
        el.disabled = anonActive;
        if (anonActive) el.value = "";
    });
}

document.addEventListener("DOMContentLoaded", () => {

    const form = document.getElementById("denunciaForm");

    //envio de formulario
    form.addEventListener("submit", async (e) => {
        e.preventDefault();

        if (!validarFormulario()) return;

        const btnSubmit = form.querySelector(".btn-primary");
        const textoOriginal = btnSubmit.textContent;
        btnSubmit.disabled = true;
        btnSubmit.textContent = "Enviando...";

        try {
            const denunciaJSON = recopilarDatosJSON();

            const response = await fetch("/api/denuncias", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify(denunciaJSON)
            });

            if (!response.ok) throw new Error("Error al enviar la denuncia");

            const resultado = await response.json();
            mostrarMensajeExito(resultado.data.codigoSeguimiento);
            form.reset();

            window.location.href = "/confirmacion?codigo=" + resultado.data.codigoSeguimiento;

        } catch (error) {
            console.error(error);
            alert("Ocurrió un error al enviar la denuncia. Inténtelo nuevamente.");
        } finally {
            btnSubmit.disabled = false;
            btnSubmit.textContent = textoOriginal;
        }
    });

    // validaciones
    function validarFormulario() {
        const anon = document.getElementById("anonSwitch").checked;
        const descripcion = form.querySelector("textarea[name='descripcion']");

        if (descripcion.value.length < 50) {
            alert("La descripción debe tener mínimo 50 caracteres.");
            descripcion.focus();
            return false;
        }

        if (!anon) {
            const requeridos = [
                "tipoDocumento",
                "numDocumento",
                "nombres",
                "apellidos",
                "email",
                "telefono"
            ];

            for (let campo of requeridos) {
                const input = form.querySelector(`[name="${campo}"]`);
                if (!input || input.value.trim() === "") {
                    alert("Complete todos los campos del denunciante o active el modo anónimo.");
                    input.focus();
                    return false;
                }
            }
        }

        return true;
    }


    // a json
    function recopilarDatosJSON() {
        const anon = document.getElementById("anonSwitch").checked;
        const formData = new FormData(form);
        const datos = {};

        formData.forEach((value, key) => {
            datos[key] = value;
        });

        // Si es anónimo, limpiar esos campos antes de enviar a Go
        if (anon) {
            datos.tipoDocumento = "";
            datos.numDocumento = "";
            datos.nombres = "";
            datos.apellidos = "";
            datos.email = "";
            datos.telefono = "";
        }

        return datos;
    }

    function mostrarMensajeExito(codigo) {
        alert(
            "Denuncia registrada exitosamente.\n\n" +
            "Código de seguimiento: " + codigo + "\n\n" +
            "Guarde su código para consultar el estado de su denuncia."
        );
    }


    const documentoInput = form.querySelector('input[name="numDocumento"]');
    if (documentoInput) {
        documentoInput.addEventListener("input", function () {
            this.value = this.value.replace(/[^0-9]/g, "");
        });
    }

    const telefonoInput = form.querySelector('input[name="telefono"]');
    if (telefonoInput) {
        telefonoInput.addEventListener("input", function () {
            this.value = this.value.replace(/[^0-9\s]/g, "");
        });
    }

});