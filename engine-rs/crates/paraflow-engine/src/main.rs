use std::{env, io, process};

fn main() {
    let args = env::args().skip(1).collect::<Vec<_>>();
    let exit_code = paraflow_engine::run(&args, &mut io::stdout(), &mut io::stderr());
    process::exit(exit_code);
}
