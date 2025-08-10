use pyo3::ffi::c_str;
use pyo3::prelude::*;
use pyo3::types::{PyBytes, PyDict, PyModule};
use std::fs;

fn main() -> PyResult<()> {
    let code = fs::read_to_string("script.py").expect("read error");

    Ok(Python::with_gil(|py| {
        let locals = PyDict::new(py);
        py.run(
            c_str!(
                    r#"
import base64
s = 'Hello Rust!'
ret = base64.b64encode(s.encode('utf-8'))
"#
                ),
            None,
            Some(&locals),
        )
            .unwrap();
        let ret = locals.get_item("ret").unwrap().unwrap();
        let b64 = ret.downcast::<PyBytes>().unwrap();
        assert_eq!(b64.as_bytes(), b"SGVsbG8gUnVzdCE=");
        println!("ret: {:?}", ret);
    }))
}
