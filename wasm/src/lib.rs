// WASM is single-threaded; mutable static for the output buffer is safe here.
#![allow(static_mut_refs)]

use std::collections::HashMap;
use std::fmt::Write as FmtWrite;

use base64::Engine as _;
use typst::diag::Warned;
use typst::foundations::{Array, Dict, Str, Value};
use typst::layout::PagedDocument;
use typst_as_lib::TypstEngine;

// ---------- Static output buffer --------------------------------------------

static mut OUTPUT: Vec<u8> = Vec::new();

// ---------- Wire format -----------------------------------------------------

// The Go caller sends a single JSON envelope:
//
//   {
//     "main":  "... typst source of the main template ...",
//     "files": { "layout.typ": "...", "other.typ": "..." },
//     "data":  { ... arbitrary caller data, injected as sys.inputs ... },
//     "fonts": [ "<base64-encoded TTF/OTF>", ... ]
//   }
//
// Go's encoding/json marshals [][]byte as an array of standard base64 strings,
// which is why fonts arrive here as Vec<String>.

#[derive(serde::Deserialize)]
struct Envelope {
    main: String,
    #[serde(default)]
    files: HashMap<String, String>,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    fonts: Vec<String>,
}

// ---------- JSON → Typst Dict conversion ------------------------------------

fn json_to_typst_value(v: &serde_json::Value) -> Value {
    match v {
        serde_json::Value::Null => Value::None,
        serde_json::Value::Bool(b) => Value::Bool(*b),
        serde_json::Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Value::Int(i)
            } else {
                Value::Float(n.as_f64().unwrap_or(0.0))
            }
        }
        serde_json::Value::String(s) => Value::Str(Str::from(s.as_str())),
        serde_json::Value::Array(arr) => {
            let mut typst_arr = Array::new();
            for item in arr {
                typst_arr.push(json_to_typst_value(item));
            }
            Value::Array(typst_arr)
        }
        serde_json::Value::Object(obj) => Value::Dict(json_object_to_typst_dict(obj)),
    }
}

fn json_object_to_typst_dict(obj: &serde_json::Map<String, serde_json::Value>) -> Dict {
    let mut dict = Dict::new();
    for (k, v) in obj {
        dict.insert(Str::from(k.as_str()), json_to_typst_value(v));
    }
    dict
}

// ---------- Memory allocation exports ----------------------------------------

#[unsafe(no_mangle)]
pub extern "C" fn alloc(len: u32) -> u32 {
    let layout = std::alloc::Layout::from_size_align(len as usize, 1).unwrap();
    unsafe { std::alloc::alloc(layout) as u32 }
}

#[unsafe(no_mangle)]
pub extern "C" fn dealloc(ptr: u32, len: u32) {
    let layout = std::alloc::Layout::from_size_align(len as usize, 1).unwrap();
    unsafe { std::alloc::dealloc(ptr as *mut u8, layout) }
}

// ---------- Exported compile function ----------------------------------------

#[unsafe(no_mangle)]
pub extern "C" fn compile(json_ptr: u32, json_len: u32) -> u64 {
    match do_compile(json_ptr, json_len) {
        Ok(result) => result,
        Err(e) => {
            let mut s = String::new();
            let _ = writeln!(s, "{e}");
            let _ = std::io::Write::write_all(&mut std::io::stderr(), s.as_bytes());
            u64::MAX
        }
    }
}

fn do_compile(json_ptr: u32, json_len: u32) -> Result<u64, Box<dyn std::error::Error>> {
    // Safety: caller guarantees the pointer/length describe a valid byte slice
    // that remains valid for the duration of this call.
    let json_bytes =
        unsafe { std::slice::from_raw_parts(json_ptr as *const u8, json_len as usize) };

    let envelope: Envelope = serde_json::from_slice(json_bytes)?;

    // Decode user-supplied fonts from base64.
    let font_data: Vec<Vec<u8>> = envelope
        .fonts
        .iter()
        .map(|s| {
            base64::engine::general_purpose::STANDARD
                .decode(s)
                .map_err(|e| format!("font base64 decode: {e}"))
        })
        .collect::<Result<_, _>>()?;
    let font_slices: Vec<&[u8]> = font_data.iter().map(|v| v.as_slice()).collect();

    // Build input dict from the "data" field.
    let input_dict = match &envelope.data {
        serde_json::Value::Object(obj) => json_object_to_typst_dict(obj),
        serde_json::Value::Null => Dict::new(),
        _ => return Err("envelope.data must be a JSON object or null".into()),
    };

    // Collect auxiliary source files for the resolver.
    let aux_files: Vec<(&str, &str)> = envelope
        .files
        .iter()
        .map(|(k, v)| (k.as_str(), v.as_str()))
        .collect();

    let engine = TypstEngine::builder()
        .main_file(envelope.main.as_str())
        .with_static_source_file_resolver(aux_files)
        .fonts(font_slices)
        .build();

    let Warned { output, .. } = engine.compile_with_input::<_, PagedDocument>(input_dict);
    let output: Result<PagedDocument, _> = output;
    let doc = output.map_err(|e| format!("typst compile error: {e:?}"))?;

    let pdf_bytes = typst_pdf::pdf(&doc, &typst_pdf::PdfOptions::default())
        .map_err(|e| format!("typst pdf error: {e:?}"))?;

    // Store in static buffer and return packed (ptr << 32 | len).
    // Safety: WASM is single-threaded; no concurrent access to OUTPUT.
    unsafe {
        OUTPUT = pdf_bytes;
        let ptr = OUTPUT.as_ptr() as u64;
        let len = OUTPUT.len() as u64;
        Ok((ptr << 32) | len)
    }
}
